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
	"net"

	"github.com/matrix-org/pinecone/types"
)

// The writer goroutine is responsible for sending traffic from
// the router to the switch.
func (r *Router) writer(conn net.Conn) {
	buf := make([]byte, types.MaxFrameSize)
	for {
		select {
		case <-r.context.Done():
			return

		default:
			func(frame *types.Frame) {
				defer frame.Done()
				n, err := frame.MarshalBinary(buf)
				if err != nil {
					r.log.Printf("frame.MarshalBinary: %s\n", err)
					return
				}
				if !bytes.Equal(buf[:len(types.FrameMagicBytes)], types.FrameMagicBytes) {
					r.log.Println("Should be sending magic bytes", types.FrameMagicBytes)
					return
				}
				if _, err = conn.Write(buf[:n]); err != nil {
					r.log.Println("s.conn.Write:", err)
					return
				}
			}(<-r.send)
		}
	}
}

// The reader goroutine is responsible for receiving traffic
// for the router from the switch.
func (r *Router) reader(conn net.Conn) {
	buf := make([]byte, types.MaxFrameSize)
	for {
		select {
		case <-r.context.Done():
			return

		default:
			n, err := conn.Read(buf)
			if err != nil {
				r.log.Println("s.conn.Read:", err)
				continue
			}

			func(frame *types.Frame) {
				defer frame.Done()
				if _, err = frame.UnmarshalBinary(buf[:n]); err != nil {
					r.log.Printf("frame.UnmarshalBinary: %s\n", err)
					return
				}

				switch frame.Type {
				case types.TypeGreedy:
					// If the frame doesn't appear as if it's meant to be for
					// us then we'll drop it.
					if !r.imprecise.Load() && !frame.Destination.EqualTo(r.Coords()) {
						//r.log.Println("Router received frame that isn't for us")
						return
					}
					r.recv <- frame.Borrow()

				case types.TypeVirtualSnake:
					// If the frame doesn't appear as if it's meant to be for
					// us then we'll drop it.
					if !r.imprecise.Load() && !frame.DestinationKey.EqualTo(r.PublicKey()) {
						//	r.log.Println("Router received frame that isn't for us")
						return
					}
					r.recv <- frame.Borrow()

				case types.TypeSource:
					// Check if the source path seems to be finished.
					if len(frame.Destination) > 0 {
						if frame.Destination[0] != 0 {
							//r.log.Println("Dropping frame that has invalid next-port")
							return
						}
						frame.Destination = frame.Destination[1:]
					}
					r.recv <- frame.Borrow()

				case types.TypeDHTRequest:
					var request types.DHTQueryRequest
					if _, err := request.UnmarshalBinary(frame.Payload); err != nil {
						r.log.Println("DHTQueryRequest.MarshalBinary:", err)
						return
					}
					r.dht.onDHTRequest(&request, frame.Source)

				case types.TypeDHTResponse:
					var response types.DHTQueryResponse
					if _, err := response.UnmarshalBinary(frame.Payload); err != nil {
						r.log.Println("DHTQueryResponse.UnmarshalBinary:", err)
						return
					}
					r.dht.onDHTResponse(&response, frame.Source)

				case types.TypePathfind, types.TypeVirtualSnakePathfind:
					if len(frame.Payload) == 0 {
						return
					}
					var pathfind types.Pathfind
					if _, err := pathfind.UnmarshalBinary(frame.Payload); err != nil {
						r.log.Println("pathfind.UnmarshalBinary:", err)
						return
					}
					if pathfind.Boundary == 0 {
						// The search has been sent to us. Now let's set the boundary and
						// send it back. This lets the other end work out how much of the
						// body was the path here and how much of it was the path back.
						signed, err := pathfind.Sign(r.private[:], 0)
						if err != nil {
							r.log.Println("pathfind.Sign:", err)
							return
						}
						signed.Boundary = uint8(len(signed.Signatures))
						buffer := bufPool.Get().(*[types.MaxFrameSize]byte)
						n, err := signed.MarshalBinary(buffer[:])
						if err != nil {
							r.log.Println("signed.MarshalBinary:", err)
							return
						}
						switch frame.Type {
						case types.TypePathfind:
							f := types.GetFrame()
							f.Destination = frame.Source
							f.Source = frame.Destination
							f.Type = types.TypePathfind
							f.Payload = append(f.Payload[:0], buffer[:n]...)
							r.send <- f
						case types.TypeVirtualSnakePathfind:
							f := types.GetFrame()
							f.DestinationKey = frame.SourceKey
							f.SourceKey = frame.DestinationKey
							f.Type = types.TypeVirtualSnakePathfind
							f.Payload = append(f.Payload[:0], buffer[:n]...)
							r.send <- f
						}
						bufPool.Put(buffer)
					} else {
						// This is a response to a search that we sent out. It will contain
						// both the path we took to the destination (before the boundary)
						// and the return path (after the boundary). We can pick which of
						// the routes was shorter to reduce stretch.
						if len(pathfind.Signatures) < int(pathfind.Boundary) {
							return
						}
						switch frame.Type {
						case types.TypePathfind:
							r.pathfinder.onPathfindResponse(&GreedyAddr{frame.Source}, pathfind)
						case types.TypeVirtualSnakePathfind:
							r.pathfinder.onPathfindResponse(frame.SourceKey, pathfind)
						}
					}
				}
			}(types.GetFrame())
		}
	}
}
