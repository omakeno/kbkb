package scheduler

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"time"
)

//go:embed web
var webFS embed.FS

// Server exposes the game over HTTP: the embedded web UI, a JSON state
// endpoint, an SSE stream, and the drop/mode commands.
type Server struct {
	Scheduler *Scheduler
}

// Handler builds the HTTP routes.
func (srv *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	web, _ := fs.Sub(webFS, "web")
	mux.Handle("/", http.FileServer(http.FS(web)))
	mux.HandleFunc("GET /api/state", srv.handleState)
	mux.HandleFunc("GET /api/events", srv.handleEvents)
	mux.HandleFunc("POST /api/drop", srv.handleDrop)
	mux.HandleFunc("POST /api/mode", srv.handleMode)
	return mux
}

func (srv *Server) writeState(w http.ResponseWriter, r *http.Request) error {
	st, err := srv.Scheduler.State(r.Context())
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(st)
}

func (srv *Server) handleState(w http.ResponseWriter, r *http.Request) {
	if err := srv.writeState(w, r); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// handleEvents streams the game state as Server-Sent Events: one event on
// connect, then one per cluster change, plus a keep-alive heartbeat.
func (srv *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch, cancel := srv.Scheduler.Subscribe()
	defer cancel()

	send := func() bool {
		st, err := srv.Scheduler.State(r.Context())
		if err != nil {
			return false
		}
		b, err := json.Marshal(st)
		if err != nil {
			return false
		}
		if _, err := fmt.Fprintf(w, "data: %s\n\n", b); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}
	if !send() {
		return
	}

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ch:
			if !send() {
				return
			}
		case <-heartbeat.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (srv *Server) handleDrop(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Placements []Placement `json:"placements"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := srv.Scheduler.Drop(r.Context(), req.Placements); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if err := srv.writeState(w, r); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (srv *Server) handleMode(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Mode string `json:"mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := srv.Scheduler.SetMode(req.Mode); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := srv.writeState(w, r); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
