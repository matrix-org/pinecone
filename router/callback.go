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
	"sync"

	"github.com/matrix-org/pinecone/types"
)

type callbacks struct {
	r            *Router
	mutex        sync.RWMutex
	connected    func(port types.SwitchPortID, publickey types.PublicKey, peertype int)
	disconnected func(port types.SwitchPortID, publickey types.PublicKey, peertype int, err error)
}

// SetConnectedCallback defines a function that will be called when a new peer
// connection is successfully established.
func (r *Router) SetConnectedCallback(f func(port types.SwitchPortID, publickey types.PublicKey, peertype int)) {
	r.callbacks.mutex.Lock()
	defer r.callbacks.mutex.Unlock()
	r.callbacks.connected = f
}

// SetDisconnectedCallback defines a function that will be called either when
// an existing peer connection disconnects. An error field contains additional
// information about why the peering connection was closed.
func (r *Router) SetDisconnectedCallback(f func(port types.SwitchPortID, publickey types.PublicKey, peertype int, err error)) {
	r.callbacks.mutex.Lock()
	defer r.callbacks.mutex.Unlock()
	r.callbacks.disconnected = f
}

func (c *callbacks) onConnected(port types.SwitchPortID, publickey types.PublicKey, peertype int) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	if c.connected != nil {
		c.connected(port, publickey, peertype)
	}
}

func (c *callbacks) onDisconnected(port types.SwitchPortID, publickey types.PublicKey, peertype int, err error) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	if c.disconnected != nil {
		c.disconnected(port, publickey, peertype, err)
	}
}
