package web

import (
	"net/http"
)

func (srv *Server) handleBuildsList(w http.ResponseWriter, r *http.Request) {
	builds, err := srv.store.ListBuilds()
	if err != nil {
		http.Error(w, "failed to list builds", http.StatusInternalServerError)
		return
	}
	data := map[string]any{"Builds": builds}
	if err := srv.tmpl.ExecuteTemplate(w, "builds_list.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (srv *Server) handleBuildDetail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	build, err := srv.store.GetBuild(id)
	if err != nil {
		http.Error(w, "build not found", http.StatusNotFound)
		return
	}
	steps, err := srv.store.ListStepResults(id)
	if err != nil {
		http.Error(w, "failed to list steps", http.StatusInternalServerError)
		return
	}
	data := map[string]any{"Build": build, "Steps": steps}
	if err := srv.tmpl.ExecuteTemplate(w, "build_detail.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
