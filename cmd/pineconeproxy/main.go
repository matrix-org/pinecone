package main

import (
	"context"
	"crypto/ed25519"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/pinecone/multicast"
	"github.com/matrix-org/pinecone/router"
	"github.com/matrix-org/pinecone/sessions"
)

type node struct {
	ctx       context.Context
	cancel    context.CancelFunc
	log       *log.Logger
	router    *router.Router
	multicast *multicast.Multicast
	sessions  *sessions.Sessions
	client    *http.Client // talks to pinecone
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())

	_, sk, err := ed25519.GenerateKey(nil)
	if err != nil {
		panic(err)
	}

	n := &node{
		ctx:    ctx,
		cancel: cancel,
		log:    log.New(os.Stdout, "", 0),
	}

	n.router = router.NewRouter(n.log, sk, false)
	n.sessions = sessions.NewSessions(n.log, n.router, []string{"matrix"})
	n.multicast = multicast.NewMulticast(n.log, n.router)
	n.multicast.Start()

	pineconeHTTP := n.sessions.Protocol("matrix").HTTP()
	pineconeHTTP.Mux().HandleFunc("/", n.handleHTTPPineconeToFederation)
	n.client = pineconeHTTP.Client()

	http.DefaultServeMux.HandleFunc("/", n.handleHTTPFederationToPinecone)

	go func() {
		_ = http.ListenAndServe(":8228", http.DefaultServeMux)
		n.cancel()
	}()

	<-ctx.Done()
}

func (n *node) handleHTTPPineconeToFederation(w http.ResponseWriter, req *http.Request) {
	if !strings.HasPrefix(req.URL.Path, "/_matrix") {
		http.Error(w, "not /_matrix", http.StatusServiceUnavailable)
		return
	}
	req.URL.Scheme = "https"
	req.URL.Host = req.Host
	resp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer resp.Body.Close()
	for k, v := range resp.Header {
		for _, vv := range v {
			w.Header().Add(k, vv)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func (n *node) handleHTTPFederationToPinecone(w http.ResponseWriter, req *http.Request) {
	if !strings.HasPrefix(req.URL.Path, "/_matrix") {
		http.Error(w, "not /_matrix", http.StatusServiceUnavailable)
		return
	}
	xmatrix := req.Header.Get("Authorization")
	_, _, host, _, _ := gomatrixserverlib.ParseAuthorization(xmatrix)
	if host == "" {
		query := req.URL.Query()
		host = gomatrixserverlib.ServerName(query.Get("_destination"))
		query.Del("_destination")
		req.URL.RawQuery = query.Encode()
		if host == "" {
			http.Error(w, "no destination supplied in request", http.StatusServiceUnavailable)
			return
		}
	}
	req.URL.Scheme = "http"
	req.Host = string(host)
	req.URL.Host = string(host)
	resp, err := n.client.Transport.RoundTrip(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer resp.Body.Close()
	for k, v := range resp.Header {
		for _, vv := range v {
			w.Header().Add(k, vv)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
	n.log.Println("Returning", resp.StatusCode, "for", req.URL.String())
}
