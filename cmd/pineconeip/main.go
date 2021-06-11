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
	"crypto/ed25519"
	"flag"
	"fmt"
	"log"
	"net"
	"os"

	"net/http"
	_ "net/http/pprof"

	"github.com/matrix-org/pinecone/cmd/pineconeip/tun"
	"github.com/matrix-org/pinecone/multicast"
	"github.com/matrix-org/pinecone/router"
)

func main() {
	pk, sk, err := ed25519.GenerateKey(nil)
	if err != nil {
		panic(err)
	}

	listen := flag.String("listen", "", "address to listen on")
	connect := flag.String("connect", "", "peer to connect to")
	flag.Parse()

	addr, err := net.ResolveTCPAddr("tcp", *listen)
	if err != nil {
		panic(err)
	}

	listener, err := net.ListenTCP("tcp", addr)
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

	pineconeRouter := router.NewRouter(logger, "router", sk, pk, nil)
	pineconeRouter.AllowImpreciseTraffic(true)
	pineconeMulticast := multicast.NewMulticast(logger, pineconeRouter)
	pineconeMulticast.Start()
	pineconeTUN, err := tun.NewTUN(pineconeRouter)
	if err != nil {
		panic(err)
	}
	_ = pineconeTUN

	tcpParams := func(conn *net.TCPConn) error {
		if err := conn.SetNoDelay(true); err != nil {
			return fmt.Errorf("conn.SetNoDelay: %w", err)
		}
		/*
			if err := conn.SetKeepAlive(true); err != nil {
				return fmt.Errorf("conn.SetKeepAlive: %w", err)
			}
			if err := conn.SetKeepAlivePeriod(time.Second); err != nil {
				return fmt.Errorf("conn.SetKeepAlivePeriod: %w", err)
			}
		*/
		if err := conn.SetLinger(0); err != nil {
			return fmt.Errorf("conn.SetLinger: %w", err)
		}
		return nil
	}

	if connect != nil && *connect != "" {
		go func() {
			addr, err := net.ResolveTCPAddr("tcp", *connect)
			if err != nil {
				panic(err)
			}

			conn, err := net.DialTCP("tcp", nil, addr)
			if err != nil {
				panic(err)
			}

			if err := tcpParams(conn); err != nil {
				panic(err)
			}

			port, err := pineconeRouter.AuthenticatedConnect(conn, "", router.PeerTypeRemote)
			if err != nil {
				panic(err)
			}

			fmt.Println("Outbound connection", conn.RemoteAddr(), "is connected to port", port)
		}()
	}

	go func() {
		fmt.Println("Listening on", listener.Addr())

		for {
			conn, err := listener.AcceptTCP()
			if err != nil {
				panic(err)
			}

			if err := tcpParams(conn); err != nil {
				panic(err)
			}

			port, err := pineconeRouter.AuthenticatedConnect(conn, "", router.PeerTypeRemote)
			if err != nil {
				panic(err)
			}

			fmt.Println("Inbound connection", conn.RemoteAddr(), "is connected to port", port)
		}
	}()

	select {}
}
