package web

import (
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"

	"github.com/ian-howell/cicada/internal/scheduler"
	"github.com/ian-howell/cicada/internal/store"
	"github.com/ian-howell/cicada/internal/webhook"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static/*
var staticFS embed.FS

// Server holds dependencies for the HTTP server.
type Server struct {
	store     *store.Store
	registry  *webhook.Registry
	scheduler *scheduler.Scheduler
	tmpl      *template.Template
	mux       *http.ServeMux
}

// New creates a Server and registers all routes.
func New(s *store.Store, registry *webhook.Registry, sched *scheduler.Scheduler) (*Server, error) {
	funcs := template.FuncMap{
		"shortID": func(s string) string {
			if len(s) > 8 {
				return s[:8]
			}
			return s
		},
		"shortSHA": func(s string) string {
			if len(s) > 7 {
				return s[:7]
			}
			return s
		},
	}
	tmpl, err := template.New("").Funcs(funcs).ParseFS(templateFS, "templates/*.html")
	if err != nil {
		return nil, err
	}

	srv := &Server{
		store:     s,
		registry:  registry,
		scheduler: sched,
		tmpl:      tmpl,
		mux:       http.NewServeMux(),
	}
	srv.registerRoutes()
	return srv, nil
}

func (srv *Server) registerRoutes() {
	staticContent, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic(fmt.Sprintf("web: failed to sub static fs: %v", err))
	}
	srv.mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticContent))))

	srv.mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/builds", http.StatusFound)
	})
	srv.mux.HandleFunc("GET /builds", srv.handleBuildsList)
	srv.mux.HandleFunc("GET /builds/{id}", srv.handleBuildDetail)
	srv.mux.HandleFunc("GET /builds/{id}/steps/{name}/log", srv.handleLogView)
	srv.mux.HandleFunc("POST /webhooks/{provider}", srv.handleWebhook)
	srv.mux.HandleFunc("GET /api/builds", srv.handleAPIBuilds)
	srv.mux.HandleFunc("GET /api/builds/{id}", srv.handleAPIBuild)
	srv.mux.HandleFunc("GET /api/builds/{id}/steps/{name}/log/stream", srv.handleLogStream)
}

// ServeHTTP implements http.Handler.
func (srv *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	srv.mux.ServeHTTP(w, r)
}
