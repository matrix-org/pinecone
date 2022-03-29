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

package main

import (
	"context"
	"crypto/ed25519"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"time"

	"net/http"
	_ "net/http/pprof"

	"github.com/matrix-org/pinecone/multicast"
	"github.com/matrix-org/pinecone/router"
)

func main() {
	_, sk, err := ed25519.GenerateKey(nil)
	if err != nil {
		panic(err)
	}

	logger := log.New(os.Stdout, "", 0)
	if hostPort := os.Getenv("PPROFLISTEN"); hostPort != "" {
		logger.Println("Starting pprof on", hostPort)
		go func() {
			_ = http.ListenAndServe(hostPort, nil)
		}()
	}

	dialer := net.Dialer{
		Timeout: time.Second * 5,
	}
	listener := net.ListenConfig{}

	pineconeRouter := router.NewRouter(logger, sk, false)
	pineconeMulticast := multicast.NewMulticast(logger, pineconeRouter)
	pineconeMulticast.Start()

	listen := flag.String("listen", "", "address to listen on")
	connect := flag.String("connect", "", "peer to connect to")
	flag.Parse()

	if connect != nil && *connect != "" {
		go func() {
			conn, err := dialer.Dial("tcp", *connect)
			if err != nil {
				panic(err)
			}

			if _, err := pineconeRouter.Connect(
				conn,
				router.ConnectionURI(*connect),
				router.ConnectionPeerType(router.PeerTypeRemote),
			); err != nil {
				panic(err)
			}

			fmt.Println("Outbound connection", conn.RemoteAddr(), "is connected")
		}()
	}

	go func() {
		listener, err := listener.Listen(context.Background(), "tcp", *listen)
		if err != nil {
			panic(err)
		}

		fmt.Println("Listening on", listener.Addr())

		for {
			conn, err := listener.Accept()
			if err != nil {
				panic(err)
			}

			if _, err := pineconeRouter.Connect(
				conn,
				router.ConnectionURI(conn.RemoteAddr().String()),
				router.ConnectionPeerType(router.PeerTypeRemote),
			); err != nil {
				panic(err)
			}

			fmt.Println("Inbound connection", conn.RemoteAddr(), "is connected")
		}
	}()

	select {}
}
