package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

func (srv *Server) handleAPIBuilds(w http.ResponseWriter, r *http.Request) {
	builds, err := srv.store.ListBuilds()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(builds)
}

func (srv *Server) handleAPIBuild(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	build, err := srv.store.GetBuild(id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(build)
}

func (srv *Server) handleLogStream(w http.ResponseWriter, r *http.Request) {
	buildID := r.PathValue("id")
	stepName := r.PathValue("name")

	results, err := srv.store.ListStepResults(buildID)
	if err != nil || len(results) == 0 {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	var logFile string
	for _, sr := range results {
		if sr.StepName == stepName {
			logFile = sr.LogFile
			break
		}
	}
	if logFile == "" {
		http.Error(w, "step not found", http.StatusNotFound)
		return
	}

	absPath := filepath.Join(srv.store.DataDir(), logFile)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	f, err := os.Open(absPath)
	if err != nil {
		// File may not exist yet if step hasn't started.
		fmt.Fprintf(w, "data: (waiting for log)\n\n")
		flusher.Flush()
		return
	}
	defer f.Close()

	buf := make([]byte, 4096)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			n, _ := f.Read(buf)
			if n > 0 {
				fmt.Fprintf(w, "data: %s\n\n", buf[:n])
				flusher.Flush()
			}
		}
	}
}
