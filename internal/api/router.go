package api

import (
	"log"
	"net/http"
	"time"
)

// Routes returns the HTTP handler with all endpoints registered.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /api/v1/files", s.handleListFiles)
	mux.HandleFunc("POST /api/v1/reload", s.handleReload)
	mux.HandleFunc("GET /api/v1/flows", s.handleFlows)

	// API documentation. Swagger UI assets are vendored and served from /docs/;
	// the trailing slash matters so the page's relative asset URLs resolve.
	mux.HandleFunc("GET /openapi.json", s.handleOpenAPI)
	mux.Handle("GET /docs/", http.StripPrefix("/docs/", swaggerHandler()))
	mux.HandleFunc("GET /docs", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/docs/", http.StatusMovedPermanently)
	})

	// Convenience redirect from root to the docs UI.
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/docs/", http.StatusFound)
	})

	return logRequests(mux)
}

// logRequests is lightweight request logging middleware.
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.RequestURI(), time.Since(start))
	})
}
