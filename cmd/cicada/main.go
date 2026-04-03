package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ianhomer/cicada/internal/model"
	"github.com/ianhomer/cicada/internal/runner"
	"github.com/ianhomer/cicada/internal/scheduler"
	"github.com/ianhomer/cicada/internal/store"
	"github.com/ianhomer/cicada/internal/web"
	"github.com/ianhomer/cicada/internal/webhook"
)

func main() {
	dataDir := flag.String("data-dir", "./data", "directory for SQLite database and log files")
	addr := flag.String("addr", ":8080", "HTTP listen address")
	flag.Parse()

	if err := os.MkdirAll(*dataDir, 0755); err != nil {
		log.Fatalf("create data dir: %v", err)
	}

	s, err := store.New(*dataDir)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer s.Close()

	r := runner.New(s, *dataDir)
	runFn := func(build *model.Build) {
		if err := r.Run(context.Background(), build); err != nil {
			log.Printf("runner error: %v", err)
		}
	}

	sched := scheduler.New(s, runFn)
	registry := webhook.NewRegistryFromEnv()

	srv, err := web.New(s, registry, sched)
	if err != nil {
		log.Fatalf("create server: %v", err)
	}

	httpSrv := &http.Server{
		Addr:    *addr,
		Handler: srv,
	}

	go func() {
		log.Printf("cicada listening on %s", *addr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http server: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(ctx); err != nil {
		log.Printf("http shutdown: %v", err)
	}
}
