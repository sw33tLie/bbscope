package server

import (
	"encoding/json"
	"net/http"

	"github.com/sw33tLie/bbscope/v2/pkg/storage"
)

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	stats, err := s.DB.GetStats(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(stats)
}

func (s *Server) handleScope(w http.ResponseWriter, r *http.Request) {
	// Parse query params for filtering
	q := r.URL.Query()
	opts := storage.ListOptions{
		Platform:      q.Get("platform"),
		ProgramFilter: q.Get("search"),
		IncludeOOS:    q.Get("include_oos") == "true",
		IncludeIgnored: q.Get("include_ignored") == "true",
	}

	entries, err := s.DB.ListEntries(r.Context(), opts)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(entries)
}

func (s *Server) handlePrograms(w http.ResponseWriter, r *http.Request) {
	programs, err := s.DB.ListPrograms(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(programs)
}

type IgnoreRequest struct {
	URL     string `json:"url"`
	Ignored bool   `json:"ignored"`
}

func (s *Server) handleIgnoreProgram(w http.ResponseWriter, r *http.Request) {
	var req IgnoreRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	
	if err := s.DB.SetProgramIgnoredStatus(r.Context(), req.URL, req.Ignored); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

type AddTargetRequest struct {
	Target      string `json:"target"`
	Category    string `json:"category"`
	ProgramURL  string `json:"program_url"`
}

func (s *Server) handleAddTarget(w http.ResponseWriter, r *http.Request) {
	var req AddTargetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	added, err := s.DB.AddCustomTarget(r.Context(), req.Target, req.Category, req.ProgramURL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	json.NewEncoder(w).Encode(map[string]bool{"added": added})
}

type RemoveTargetRequest struct {
	Target     string `json:"target"`
	Category   string `json:"category"`
	ProgramURL string `json:"program_url"`
}

func (s *Server) handleRemoveTarget(w http.ResponseWriter, r *http.Request) {
	var req RemoveTargetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := s.DB.RemoveCustomTarget(r.Context(), req.Target, req.Category, req.ProgramURL); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

