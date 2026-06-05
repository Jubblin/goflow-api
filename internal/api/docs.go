package api

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed openapi.json
var openapiSpec []byte

// swaggerFiles holds the vendored Swagger UI assets (CSS/JS + index.html) so
// the docs page works fully offline, with no CDN dependency.
//
//go:embed swaggerui
var swaggerFiles embed.FS

// swaggerHandler serves the Swagger UI assets rooted at the swaggerui directory.
func swaggerHandler() http.Handler {
	sub, err := fs.Sub(swaggerFiles, "swaggerui")
	if err != nil {
		panic(err) // embedded path is constant; this cannot fail at runtime
	}
	return http.FileServer(http.FS(sub))
}

// handleOpenAPI serves the embedded OpenAPI 3 specification.
func (s *Server) handleOpenAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(openapiSpec)
}
