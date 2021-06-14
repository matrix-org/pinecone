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
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/pprof"
	"time"

	"github.com/matrix-org/pinecone/types"
)

// DEBUG STATISTICS
func (r *Router) startManhole() {
	mux := http.NewServeMux()
	mux.Handle("/debug/pprof/goroutine", pprof.Handler("goroutine"))
	mux.Handle("/debug/pprof/heap", pprof.Handler("heap"))
	mux.Handle("/debug/pprof/profile", pprof.Handler("profile"))
	mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		results := map[string]interface{}{}
		results["self"] = map[string]string{
			"public_key": r.PublicKey().String(),
			"coords":     r.Coords().String(),
		}
		results["tree"] = map[string]string{
			"root":      r.tree.Root().RootPublicKey.String(),
			"last_seen": time.Since(r.tree.Root().at).String(),
		}
		ports := map[types.SwitchPortID]map[string]interface{}{}
		for _, p := range r.activePorts() {
			p.mutex.RLock()
			ports[p.port] = map[string]interface{}{
				"zone":                  p.zone,
				"public_key":            p.public.String(),
				"coords":                p.coords.String(),
				"peer_type":             p.peertype,
				"queued_proto":          fmt.Sprintf("%d/%d", p.protoOut.queuecount(), p.protoOut.queuesize()),
				"queued_traffic":        fmt.Sprintf("%d/%d", p.trafficOut.queuecount(), p.trafficOut.queuesize()),
				"tx_proto_successful":   p.statistics.txProtoSuccessful.Load(),
				"tx_proto_dropped":      p.statistics.txProtoDropped.Load(),
				"tx_traffic_successful": p.statistics.txTrafficSuccessful.Load(),
				"tx_traffic_dropped":    p.statistics.txTrafficDropped.Load(),
			}
			if ann := p.lastAnnouncement(); ann != nil {
				ports[p.port]["root"] = ann.RootPublicKey.String()
				ports[p.port]["last_seen"] = time.Since(ann.at).String()
			}
			p.mutex.RUnlock()
		}
		results["ports"] = ports

		r.snake.tableMutex.RLock()
		results["snake"] = map[string]interface{}{
			"predecessor": r.snake.descending(),
			"successor":   r.snake.ascending(),
			"table":       r.snake.table,
		}
		b, err := json.MarshalIndent(results, "", "  ")
		r.snake.tableMutex.RUnlock()

		if err != nil {
			w.WriteHeader(500)
			r.log.Println("Failed to marshal manhole JSON:", err)
			return
		}
		_, _ = w.Write(b)
	})
	if err := http.ListenAndServe(":64000", mux); err != nil {
		r.log.Println("Failed to setup manhole:", err)
	}
}

func (e *virtualSnakeNeighbour) MarshalJSON() ([]byte, error) {
	out := map[string]interface{}{
		"public_key": e.PublicKey.String(),
		"port":       e.Port,
		"age":        time.Since(e.LastSeen).String(),
	}
	return json.Marshal(out)
}

func (e virtualSnakeTable) MarshalJSON() ([]byte, error) {
	out := []map[string]interface{}{}
	for key, value := range e {
		entry := map[string]interface{}{
			"source_key":  key.PublicKey.String(),
			"source_port": value.SourcePort,
			"path_id":     key.PathID,
			"age":         time.Since(value.LastSeen).String(),
		}
		//if time.Since(value.LastSeen) > virtualSnakePathExpiryPeriod {
		//	entry["expired"] = true
		//}
		out = append(out, entry)
	}
	return json.Marshal(out)
}

func (e virtualSnakeEntry) MarshalJSON() ([]byte, error) {
	out := map[string]interface{}{
		"age":         time.Since(e.LastSeen).String(),
		"source_port": e.SourcePort,
	}
	return json.Marshal(out)
}
