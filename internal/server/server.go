package server

import (
	"embed"
	"io/fs"
	"log"
	"net/http"

	"github.com/sw33tLie/bbscope/v2/pkg/storage"
)

//go:embed web
var WebFS embed.FS

type Server struct {
	DB       *storage.DB
	Username string
	Password string
}

func New(db *storage.DB, user, pass string) *Server {
	return &Server{
		DB:       db,
		Username: user,
		Password: pass,
	}
}

func (s *Server) Start(addr string) error {
	mux := http.NewServeMux()

	// API Group
	mux.HandleFunc("GET /api/stats", s.basicAuth(s.handleStats))
	mux.HandleFunc("GET /api/scope", s.basicAuth(s.handleScope))
	mux.HandleFunc("GET /api/programs", s.basicAuth(s.handlePrograms))
	mux.HandleFunc("POST /api/programs/ignore", s.basicAuth(s.handleIgnoreProgram))
	mux.HandleFunc("POST /api/targets", s.basicAuth(s.handleAddTarget))
	mux.HandleFunc("DELETE /api/targets", s.basicAuth(s.handleRemoveTarget))

	// Static Files
	webRoot, err := fs.Sub(WebFS, "web")
	if err != nil {
		return err
	}
	fileServer := http.FileServer(http.FS(webRoot))
	mux.Handle("/", s.basicAuthMiddlewareForStatic(fileServer))

	log.Printf("Starting server on %s", addr)
	return http.ListenAndServe(addr, mux)
}

func (s *Server) basicAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.Username == "" && s.Password == "" {
			next(w, r)
			return
		}
		user, pass, ok := r.BasicAuth()
		if !ok || user != s.Username || pass != s.Password {
			w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func (s *Server) basicAuthMiddlewareForStatic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.Username == "" && s.Password == "" {
			next.ServeHTTP(w, r)
			return
		}
		user, pass, ok := r.BasicAuth()
		if !ok || user != s.Username || pass != s.Password {
			w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
