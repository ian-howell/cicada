package web

import (
	"net/http"
	"os"
	"path/filepath"
)

func (srv *Server) handleLogView(w http.ResponseWriter, r *http.Request) {
	buildID := r.PathValue("id")
	stepName := r.PathValue("name")

	results, err := srv.store.ListStepResults(buildID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	var logFile string
	var live bool
	for _, sr := range results {
		if sr.StepName == stepName {
			logFile = sr.LogFile
			live = sr.Status == "running" || sr.Status == "pending"
			break
		}
	}
	if logFile == "" {
		http.Error(w, "step not found", http.StatusNotFound)
		return
	}

	absPath := filepath.Join(srv.store.DataDir(), logFile)
	content, _ := os.ReadFile(absPath) // best-effort; empty if not yet written

	data := map[string]any{
		"BuildID":    buildID,
		"StepName":   stepName,
		"LogContent": string(content),
		"Live":       live,
	}
	if err := srv.tmpl.ExecuteTemplate(w, "log_view.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
