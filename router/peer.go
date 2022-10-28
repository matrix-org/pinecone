// Copyright 2021 The Matrix.org Foundation C.I.C.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package router

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/Arceliar/phony"
	"github.com/matrix-org/pinecone/types"
	"go.uber.org/atomic"
)

// NOTE: Functions prefixed with an underscore (_) are only safe to be called
// from the actor that owns them, in order to prevent data races.

// Lower numbers for these consts are typically faster connections.
const ( // These need to be a simple int type for gobind/gomobile to export them...
	PeerTypePipe int = iota
	PeerTypeMulticast
	PeerTypeBonjour
	PeerTypeRemote
	PeerTypeBluetooth
)

// peer contains information about a given active peering. There are two
// actors - a read actor (which is responsible for reading frames from the
// peering) and a write actor (which is responsible for writing frames to
// the peering). Having separate actors allows reads and writes to take
// place concurrently.
type peer struct {
	reader     phony.Inbox
	writer     phony.Inbox
	router     *Router
	port       types.SwitchPortID // Not mutated after peer setup.
	context    context.Context    // Not mutated after peer setup.
	cancel     context.CancelFunc // Not mutated after peer setup.
	conn       net.Conn           // Not mutated after peer setup.
	uri        ConnectionURI      // Not mutated after peer setup.
	zone       ConnectionZone     // Not mutated after peer setup.
	peertype   ConnectionPeerType // Not mutated after peer setup.
	public     types.PublicKey    // Not mutated after peer setup.
	keepalives bool               // Not mutated after peer setup.
	started    atomic.Bool        // Thread-safe toggle for marking a peer as down.
	proto      queue              // Thread-safe queue for outbound protocol messages.
	traffic    queue              // Thread-safe queue for outbound traffic messages.
	statistics struct {
		phony.Inbox
		_bytesRxProto   uint64
		_bytesRxTraffic uint64
		_bytesTxProto   uint64
		_bytesTxTraffic uint64
	}
}

func (p *peer) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Port      types.SwitchPortID `json:"port"`
		PublicKey types.PublicKey    `json:"public_key"`
	}{
		Port:      p.port,
		PublicKey: p.public,
	})
}

func (p *peer) String() string { // to make sim less ugly
	if p == nil {
		return "nil"
	}
	if p.port == 0 {
		return "local"
	}
	return fmt.Sprintf("%d", p.port)
}

func (p *peer) ClearBandwidthCounters() {
	phony.Block(&p.statistics, func() {
		p.statistics._bytesRxProto = 0
		p.statistics._bytesRxTraffic = 0
		p.statistics._bytesTxProto = 0
		p.statistics._bytesTxTraffic = 0
	})
}

// send queues a frame to be sent to this peer. It is safe to be called from
// other actors. The frame will be allocated to the correct queue automatically
// depending on whether it is a protocol frame or a traffic frame. This function
// will return true if the message was correctly queued or false if it was dropped,
// i.e. due to the queue overflowing.
func (p *peer) send(f *types.Frame) bool {
	var q queue
	if f.Type.IsTraffic() {
		q = p.traffic
	} else {
		q = p.proto
	}
	if q == nil {
		return false
	}
	return q.push(f)
}

// stop will immediately mark a port as offline, before dispatching a task to
// the state actor to clean up the peering. Once the peering has been stopped,
// it will immediately be marked as unsuitable in next-hop or parent selection
// candidates. A task will then be dispatched to the state actor in order to
// clean up. It is safe to call this function more than once although only the
// first call will have any effect.
func (p *peer) stop(err error) {
	// The atomic switch here immediately makes sure that the port won't be
	// used. Then we'll cancel the context and reduce the connection count.
	// Using a compare-and-swap ensures that we only act upon the stop call
	// once for a given peering.
	if !p.started.CAS(true, false) {
		return
	}

	// Cancel the context, which will stop at the next iteration of the reader
	// and writer actor function calls.
	p.cancel()

	// Decrease the connection count for this peer in this zone. The multicast
	// code uses this to determine whether we are already connected to a peer in
	// a given zone and to ignore beacons from them if we are.
	index := hex.EncodeToString(p.public[:]) + string(p.zone)
	if v, ok := p.router.active.Load(index); ok && v.(*atomic.Uint64).Dec() == 0 {
		p.router.active.Delete(index)
	}

	p.ClearBandwidthCounters()

	// Next we'll send a message to the state inbox in order to clean up.
	p.router.state.Act(nil, func() {
		// Make sure that the connection is closed.
		if p.conn != nil {
			_ = p.conn.Close()
		}

		// Drop all of the frames that are sitting in this peer's queues, since there
		// is no way to send them at this point.
		if p.proto != nil {
			p.proto.reset()
		}
		if p.traffic != nil {
			p.traffic.reset()
		}

		// Notify the tree and SNEK that the port was disconnected.: This triggers
		// tearing down of paths and possible tree re-parenting.
		p.router.state._portDisconnected(p)

		// Find the port entry and clean it up.
		for i, rp := range p.router.state._peers {
			if rp == p {
				p.router.state._removePeer(types.SwitchPortID(i))
				break
			}
		}

		// Finally, yell about the disconnection in the logs.
		if err != nil {
			p.router.log.Println("Disconnected from peer", p.public.String(), "on port", p.port, "due to error:", err)
		} else {
			p.router.log.Println("Disconnected from peer", p.public.String(), "on port", p.port)
		}
	})
}

// _write waits for packets to arrive in one of the peer queues and writes
// them to the peering connection. This function must be called from the
// peer's writer actor only.
func (p *peer) _write() {
	// If the peering has stopped then we should give up.
	if !p.started.Load() {
		return
	}
	var frame *types.Frame

	// The keepalive function will return a channel that either matches the
	// keepalive interval (if enabled) or blocks forever (if disabled).
	keepalive := func() <-chan time.Time {
		if !p.keepalives {
			return make(chan time.Time)
		}
		return time.After(peerKeepaliveInterval)
	}

	// Wait for some work to do.
	select {
	case <-p.context.Done():
		// The peer context has been cancelled, which implies that the port
		// has just been stopped.
		return
	case frame = <-p.proto.pop():
		// A protocol packet is ready to send.
		p.proto.ack()
	default:
		select {
		case <-p.context.Done():
			// The peer context has been cancelled, which implies that the port
			// has just been stopped.
			return
		case frame = <-p.proto.pop():
			// A protocol packet is ready to send.
			p.proto.ack()
		case frame = <-p.traffic.pop():
			// A protocol packet is ready to send.
			p.traffic.ack()
		case <-keepalive():
			// Nothing else happened but we reached the keepalive interval, so
			// we will generate a keepalive frame to send instead.
			frame = getFrame()
			frame.Type = types.TypeKeepalive
		}
	}

	// If the frame is `nil` at this point, it's probably because the queues
	// were reset. This *shouldn't* happen at this stage but the guard doesn't
	// hurt.
	if frame == nil {
		p.stop(fmt.Errorf("queue reset"))
		return
	}
	defer framePool.Put(frame)

	// We might have been waiting for a little while for one of the above
	// cases to happen, so let's check one more time that the peering wasn't
	// stopped before we try to marshal and send the frame.
	if !p.started.Load() {
		return
	}

	// Marshal the frame.
	buf := frameBufferPool.Get().(*[types.MaxFrameSize]byte)
	defer frameBufferPool.Put(buf)
	n, err := frame.MarshalBinary(buf[:])
	if err != nil {
		p.stop(fmt.Errorf("frame.MarshalBinary: %w", err))
		return
	}

	// If keepalives are enabled then we should set a write deadline to ensure
	// that the write doesn't block for too long. We don't do this when keepalives
	// are disabled, which allows writes to take longer.
	if p.keepalives {
		if err := p.conn.SetWriteDeadline(time.Now().Add(peerKeepaliveInterval)); err != nil {
			p.stop(fmt.Errorf("p.conn.SetWriteDeadline: %w", err))
			return
		}
	}

	// Write the frame to the peering.
	if frame.Type.IsTraffic() {
		phony.Block(&p.statistics, func() {
			p.statistics._bytesTxTraffic += uint64(n)
		})
	} else {
		phony.Block(&p.statistics, func() {
			p.statistics._bytesTxProto += uint64(n)
		})
	}

	wn, err := p.conn.Write(buf[:n])
	if err != nil {
		p.stop(fmt.Errorf("p.conn.Write: %w", err))
		return
	}

	// Check that we wrote the number of bytes that we were expecting to write.
	// If we didn't then that implies that something went wrong, so shut down the
	// peering.
	if wn != n {
		p.stop(fmt.Errorf("p.conn.Write length %d != %d", wn, n))
		return
	}

	// If keepalives are enabled then we should reset the write deadline.
	if p.keepalives {
		if err := p.conn.SetWriteDeadline(time.Time{}); err != nil {
			p.stop(fmt.Errorf("p.conn.SetWriteDeadline: %w", err))
			return
		}
	}

	// This is effectively a recursive call to queue up the next write into
	// the actor inbox.
	p.writer.Act(nil, p._write)
}

// _read waits for packets to arrive from the peering and then handles
// them appropriate. This function must be called from the peer's reader
// actor only.
func (p *peer) _read() {
	// If the peering has stopped then we should give up.
	if !p.started.Load() {
		return
	}
	b := frameBufferPool.Get().(*[types.MaxFrameSize]byte)
	defer frameBufferPool.Put(b)

	// If keepalives are enabled then we should set a read deadline to ensure
	// that the read doesn't block for too long. If we wait for a packet for too long
	// then we assume the remote peer is dead, as they should have sent us a keepalive
	// packet by then.
	if p.keepalives {
		if err := p.conn.SetReadDeadline(time.Now().Add(peerKeepaliveTimeout)); err != nil {
			p.stop(fmt.Errorf("p.conn.SetReadDeadline: %w", err))
			return
		}
	}

	// Wait for the packet to arrive from the remote peer and read only enough bytes to
	// get the header. This will tell us how much more we need to read to get the rest
	// of the frame.
	var isProtoTraffic bool
	{
		n, err := io.ReadFull(p.conn, b[:types.FrameHeaderLength])
		if err != nil {
			p.stop(fmt.Errorf("io.ReadFull Initial: %w", err))
			return
		}
		isProtoTraffic = !types.FrameType(b[5]).IsTraffic()

		if isProtoTraffic {
			phony.Block(&p.statistics, func() {
				p.statistics._bytesRxProto += uint64(n)
			})
		} else {
			phony.Block(&p.statistics, func() {
				p.statistics._bytesRxTraffic += uint64(n)
			})
		}
	}

	// Check for the presence of the magic bytes at the beginning of the frame. If they
	// are missing then something is wrong — either they sent us garbage or the offsets
	// in one of the previous packets was incorrect.
	if !bytes.Equal(b[:4], types.FrameMagicBytes) {
		p.stop(fmt.Errorf("missing magic bytes"))
		return
	}

	// Now read the rest of the packet. If something goes wrong with this then we will
	// assume that either the length given to us earlier was incorrect, or something else
	// is wrong with the peering, so we will stop the peering in either case.
	expecting := int(binary.BigEndian.Uint16(b[types.FrameHeaderLength-2 : types.FrameHeaderLength]))
	n, err := io.ReadFull(p.conn, b[types.FrameHeaderLength:expecting])
	if err != nil {
		p.stop(fmt.Errorf("io.ReadFull Remaining: %w", err))
		return
	}

	if isProtoTraffic {
		phony.Block(&p.statistics, func() {
			p.statistics._bytesRxProto += uint64(n)
		})
	} else {
		phony.Block(&p.statistics, func() {
			p.statistics._bytesRxTraffic += uint64(n)
		})
	}

	// If keepalives are disabled then we can reset the read deadline again.
	if p.keepalives {
		if err := p.conn.SetReadDeadline(time.Time{}); err != nil {
			p.stop(fmt.Errorf("conn.SetReadDeadline: %w", err))
			return
		}
	}

	// We might have been waiting for a little while for the above to yield a
	// new frame, so let's check one more time that the peering wasn't stopped
	// before we try to unmarshal and handle the frame.
	if !p.started.Load() {
		return
	}

	// Check that we read the number of bytes that we were expecting to read.
	// If we didn't then that implies that something went wrong, so shut down the
	// peering.
	if n < expecting-types.FrameHeaderLength {
		p.stop(fmt.Errorf("expecting %d bytes but got %d bytes", expecting, n))
		return
	}

	// Unmarshal the frame.
	f := getFrame()
	if _, err := f.UnmarshalBinary(b[:n+types.FrameHeaderLength]); err != nil {
		p.stop(fmt.Errorf("f.UnmarshalBinary: %w", err))
		return
	}

	// Send the frame across to the state actor to be handled/forwarded.
	p.router.state.Act(&p.reader, func() {
		if err := p.router.state._forward(p, f); err != nil {
			p.stop(fmt.Errorf("p.router.state._forward: %w", err))
			return
		}
	})

	// This is effectively a recursive call to queue up the next read into
	// the actor inbox.
	p.reader.Act(nil, p._read)
}

func (p *peer) _coords() (types.Coordinates, error) {
	var err error
	var coords types.Coordinates

	if p == p.router.local {
		coords = p.router.state._coords()
	} else {
		if announcement, ok := p.router.state._announcements[p]; ok {
			coords = announcement.PeerCoords()
		} else {
			err = fmt.Errorf("no root announcement found for peer")
		}
	}

	return coords, err
}
