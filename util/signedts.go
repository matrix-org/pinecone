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

package util

import (
	"crypto/ed25519"
	"fmt"
	"os"
	"time"

	"github.com/matrix-org/pinecone/types"
)

func SignedTimestamp(private types.PrivateKey) ([]byte, error) {
	ts, err := types.Varu64(time.Now().Unix()).MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("types.Varu64.MarshalBinary: %w", err)
	}
	if _, ok := os.LookupEnv("PINECONE_DISABLE_SIGNATURES"); !ok {
		ts = append(ts, ed25519.Sign(private[:], ts)...)
	} else {
		ts = append(ts, make([]byte, ed25519.SignatureSize)...)
	}
	return ts, nil
}

func VerifySignedTimestamp(public types.PublicKey, payload []byte) bool {
	if len(payload) < ed25519.SignatureSize+1 {
		return false
	}
	var ts types.Varu64
	if err := ts.UnmarshalBinary(payload); err != nil {
		return false
	}
	offset := ts.Length()
	if _, ok := os.LookupEnv("PINECONE_DISABLE_SIGNATURES"); !ok {
		if !ed25519.Verify(public[:], payload[:offset], payload[offset:]) {
			return false
		}
	}
	if time.Since(time.Unix(int64(ts), 0)) > time.Minute {
		return false
	}
	return true
}
