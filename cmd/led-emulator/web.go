package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

//go:embed index.html
var indexHTML []byte

// minFrameGap caps the rate states are sent to each browser.
const minFrameGap = 8 * time.Millisecond

func newServer(panels []*Panel, hub *Hub) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		data, err := json.Marshal(states(panels))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(bytes.Replace(indexHTML, []byte("/*INITIAL*/null"), data, 1))
	})
	// /events streams every panel's state as server-sent events, one event
	// whenever any of them changes.
	mux.HandleFunc("GET /events", func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		keepalive := time.NewTicker(15 * time.Second)
		defer keepalive.Stop()
		for {
			changed := hub.Changed()
			data, err := json.Marshal(states(panels))
			if err != nil {
				return
			}
			if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
				return
			}
			flusher.Flush()
			time.Sleep(minFrameGap)
			for waiting := true; waiting; {
				select {
				case <-r.Context().Done():
					return
				case <-changed:
					waiting = false
				case <-keepalive.C:
					if _, err := fmt.Fprint(w, ": keepalive\n\n"); err != nil {
						return
					}
					flusher.Flush()
				}
			}
		}
	})
	return mux
}

func states(panels []*Panel) []PanelState {
	s := make([]PanelState, len(panels))
	for i, p := range panels {
		s[i] = p.State()
	}
	return s
}
