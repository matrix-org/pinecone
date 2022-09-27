package main

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/pinecone/multicast"
	"github.com/matrix-org/pinecone/router"
	"github.com/matrix-org/pinecone/sessions"
	"github.com/matrix-org/pinecone/types"
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

	n.router = router.NewRouter(n.log, sk)
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
	pk, ok := tryHostnameFromHostHeader(req)
	if !ok {
		pk, ok = tryHostnameFromDestinationHeader(req)
		if !ok {
			pk, ok = tryHostnameFromQueryString(req)
			if !ok {
				http.Error(w, "no destination supplied in request", http.StatusServiceUnavailable)
				return
			} else {
				n.log.Println("Query string:", pk.String())
			}
		} else {
			n.log.Println("Destination header:", pk.String())
		}
	} else {
		n.log.Println("Host header:", pk.String())
	}
	req.URL.Scheme = "http"
	req.Host = pk.String()
	req.URL.Host = pk.String()
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

func tryHostnameFromDestinationHeader(req *http.Request) (types.PublicKey, bool) {
	xmatrix := req.Header.Get("Authorization")
	_, _, host, _, _ := gomatrixserverlib.ParseAuthorization(xmatrix)
	return tryDecodeHex(string(host))
}

func tryHostnameFromHostHeader(req *http.Request) (types.PublicKey, bool) {
	if !strings.HasSuffix(req.Host, ".pinecone.matrix.org") {
		return types.PublicKey{}, false
	}
	return tryDecodeHex(strings.TrimSuffix(req.Host, ".pinecone.matrix.org"))
}

func tryHostnameFromQueryString(req *http.Request) (types.PublicKey, bool) {
	query := req.URL.Query()
	host := query.Get("_destination")
	query.Del("_destination")
	req.URL.RawQuery = query.Encode()
	return tryDecodeHex(host)
}

func tryDecodeHex(h string) (types.PublicKey, bool) {
	b, err := hex.DecodeString(h)
	if err != nil {
		return types.PublicKey{}, false
	}
	if len(b) != ed25519.PublicKeySize {
		return types.PublicKey{}, false
	}
	var pk types.PublicKey
	copy(pk[:], b)
	return pk, true
}
