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
	"bufio"
	"io"
)

type BufferedRWC struct {
	r *bufio.Reader
	w *bufio.Writer
	io.ReadWriteCloser
}

func NewBufferedRWC(c io.ReadWriteCloser) BufferedRWC {
	return BufferedRWC{bufio.NewReader(c), bufio.NewWriter(c), c}
}

func NewBufferedRWCSize(c io.ReadWriteCloser, n int) BufferedRWC {
	return BufferedRWC{bufio.NewReaderSize(c, n), bufio.NewWriterSize(c, n), c}
}

func (b BufferedRWC) Peek(n int) ([]byte, error) {
	return b.r.Peek(n)
}

func (b BufferedRWC) Read(p []byte) (int, error) {
	return b.r.Read(p)
}

func (b BufferedRWC) Write(p []byte) (int, error) {
	return b.w.Write(p)
}

func (b BufferedRWC) Flush() error {
	return b.w.Flush()
}

func (b BufferedRWC) Discard(n int) (int, error) {
	return b.r.Discard(n)
}
