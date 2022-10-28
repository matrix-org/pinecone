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
	"os/signal"
	"strings"
	"syscall"

	"net/http"
	_ "net/http/pprof"

	"github.com/gorilla/websocket"
	"github.com/matrix-org/pinecone/connections"
	"github.com/matrix-org/pinecone/multicast"
	"github.com/matrix-org/pinecone/router"
	"github.com/matrix-org/pinecone/util"
)

func main() {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

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

	listener := net.ListenConfig{}

	pineconeRouter := router.NewRouter(logger, sk, router.RouterOptionBlackhole(true))
	pineconeMulticast := multicast.NewMulticast(logger, pineconeRouter)
	pineconeMulticast.Start()
	pineconeManager := connections.NewConnectionManager(pineconeRouter, nil)

	listentcp := flag.String("listen", ":0", "address to listen for TCP connections")
	listenws := flag.String("listenws", ":0", "address to listen for WebSockets connections")
	connect := flag.String("connect", "", "peers to connect to")
	manhole := flag.Bool("manhole", false, "enable the manhole (requires WebSocket listener to be active)")
	flag.Parse()

	if connect != nil && *connect != "" {
		for _, uri := range strings.Split(*connect, ",") {
			pineconeManager.AddPeer(strings.TrimSpace(uri))
		}
	}

	if listenws != nil && *listenws != "" {
		go func() {
			var upgrader = websocket.Upgrader{}
			http.DefaultServeMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
				conn, err := upgrader.Upgrade(w, r, nil)
				if err != nil {
					log.Println(err)
					return
				}

				if _, err := pineconeRouter.Connect(
					util.WrapWebSocketConn(conn),
					router.ConnectionURI(conn.RemoteAddr().String()),
					router.ConnectionPeerType(router.PeerTypeRemote),
					router.ConnectionZone("websocket"),
				); err != nil {
					fmt.Println("Inbound WS connection", conn.RemoteAddr(), "error:", err)
					_ = conn.Close()
				} else {
					fmt.Println("Inbound WS connection", conn.RemoteAddr(), "is connected")
				}
			})

			if *manhole {
				fmt.Println("Enabling manhole on HTTP listener")
				http.DefaultServeMux.HandleFunc("/manhole", func(w http.ResponseWriter, r *http.Request) {
					pineconeRouter.ManholeHandler(w, r)
				})
			}

			listener, err := listener.Listen(context.Background(), "tcp", *listenws)
			if err != nil {
				panic(err)
			}

			fmt.Printf("Listening for WebSockets on http://%s\n", listener.Addr())

			if err := http.Serve(listener, http.DefaultServeMux); err != nil {
				panic(err)
			}
		}()
	}

	if listentcp != nil && *listentcp != "" {
		go func() {
			listener, err := listener.Listen(context.Background(), "tcp", *listentcp)
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
					fmt.Println("Inbound TCP connection", conn.RemoteAddr(), "error:", err)
					_ = conn.Close()
				} else {
					fmt.Println("Inbound TCP connection", conn.RemoteAddr(), "is connected")
				}
			}
		}()
	}

	<-sigs
}
