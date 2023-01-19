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
	"os/signal"
	"runtime/pprof"
	"syscall"
	"time"

	"net/http"

	"github.com/matrix-org/pinecone/cmd/pineconeip/tun"
	"github.com/matrix-org/pinecone/connections"
	"github.com/matrix-org/pinecone/multicast"
	"github.com/matrix-org/pinecone/router"
)

func main() {
	_, sk, err := ed25519.GenerateKey(nil)
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
		go func() {
			listener, err := net.Listen("tcp", hostPort)
			if err != nil {
				panic(err)
			}
			logger.Println("Starting pprof on", listener.Addr())
			if err := http.Serve(listener, nil); err != nil {
				panic(err)
			}
		}()
	}

	pineconeRouter := router.NewRouter(logger, sk)
	pineconeMulticast := multicast.NewMulticast(logger, pineconeRouter)
	pineconeMulticast.Start()
	pineconeManager := connections.NewConnectionManager(pineconeRouter, nil)
	pineconeTUN, err := tun.NewTUN(pineconeRouter)
	if err != nil {
		panic(err)
	}
	_ = pineconeTUN

	if connect != nil && *connect != "" {
		pineconeManager.AddPeer(*connect)
	}

	go func() {
		fmt.Println("Listening on", listener.Addr())

		for {
			conn, err := listener.AcceptTCP()
			if err != nil {
				panic(err)
			}

			port, err := pineconeRouter.Connect(
				conn,
				router.ConnectionURI(conn.RemoteAddr().String()),
				router.ConnectionPeerType(router.PeerTypeRemote),
			)
			if err != nil {
				panic(err)
			}

			fmt.Println("Inbound connection", conn.RemoteAddr(), "is connected to port", port)
		}
	}()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGUSR1, syscall.SIGUSR2)
	for {
		switch <-sigs {
		case syscall.SIGUSR1:
			fn := fmt.Sprintf("/tmp/profile.%d", os.Getpid())
			logger.Println("Requested profile:", fn)
			fp, err := os.Create(fn)
			if err != nil {
				logger.Println("Failed to create profile:", err)
				return
			}
			defer fp.Close()
			if err := pprof.StartCPUProfile(fp); err != nil {
				logger.Println("Failed to start profiling:", err)
				return
			}
			time.AfterFunc(time.Second*10, func() {
				pprof.StopCPUProfile()
				logger.Println("Profile written:", fn)
			})
		}
	}
}
