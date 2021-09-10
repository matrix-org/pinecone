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

const (
	// reserved = 1
	capabilityVirtualSnake     = 2
	capabilityHardState        = 4
	capabilityPathIDs          = 8
	capabilityRootInUpdates    = 16
	capabilityNewHeaders       = 32
	capabilitySlowRootInterval = 64
)

const ourVersion = 0
const ourCapabilities = capabilityVirtualSnake | capabilityHardState | capabilityPathIDs | capabilityRootInUpdates | capabilityNewHeaders | capabilitySlowRootInterval
