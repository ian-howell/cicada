package web

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/ianhomer/cicada/internal/runner"
)

func (srv *Server) handleWebhook(w http.ResponseWriter, r *http.Request) {
	providerName := r.PathValue("provider")
	provider, ok := srv.registry.Get(providerName)
	if !ok {
		http.Error(w, "unknown provider", http.StatusNotFound)
		return
	}

	event, err := provider.ParseWebhook(r)
	if err != nil {
		log.Printf("webhook: parse error for %s: %v", providerName, err)
		http.Error(w, "invalid webhook", http.StatusBadRequest)
		return
	}

	log.Printf("webhook: received %s event from %s (ref=%s sha=%s)", event.Type, event.Repo, event.Ref, event.CommitSHA)

	go func() {
		ctx := context.Background() // don't use r.Context() — it's cancelled when handler returns
		// Clone the repo to discover pipelines and trigger builds.
		cloneDir, err := os.MkdirTemp("", "cicada-discover-*")
		if err != nil {
			log.Printf("webhook: failed to create temp dir: %v", err)
			return
		}
		defer os.RemoveAll(cloneDir)

		if err := runner.CloneRepo(ctx, event.CloneURL, event.CommitSHA, cloneDir); err != nil {
			log.Printf("webhook: clone failed: %v", err)
			return
		}

		if err := srv.scheduler.DispatchForgeEvent(event, cloneDir); err != nil {
			log.Printf("webhook: dispatch failed: %v", err)
		}
	}()

	w.WriteHeader(http.StatusNoContent)
}
