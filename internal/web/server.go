package web

import (
	"embed"
	"html/template"
	"io/fs"
	"net/http"

	"github.com/ianhomer/cicada/internal/scheduler"
	"github.com/ianhomer/cicada/internal/store"
	"github.com/ianhomer/cicada/internal/webhook"
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
	tmpl, err := template.ParseFS(templateFS, "templates/*.html")
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
	staticContent, _ := fs.Sub(staticFS, "static")
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
