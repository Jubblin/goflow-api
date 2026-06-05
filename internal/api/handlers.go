// Package api wires the HTTP layer for the goflow2 REST service.
package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/richardw/goflow-api/internal/ingest"
	"github.com/richardw/goflow-api/internal/store"
)

const (
	defaultPageSize = 50
	maxPageSize     = 500
)

// Server holds the dependencies shared by the HTTP handlers.
type Server struct {
	store  *store.Store
	loader *ingest.Loader
}

// NewServer builds an API server over the given store and loader.
func NewServer(s *store.Store, l *ingest.Loader) *Server {
	return &Server{store: s, loader: l}
}

// pagination is the metadata envelope returned alongside flow results.
type pagination struct {
	Page       int   `json:"page"`
	PageSize   int   `json:"page_size"`
	Total      int64 `json:"total"`
	TotalPages int64 `json:"total_pages"`
}

// flowsResponse is the top-level envelope for GET /api/v1/flows.
type flowsResponse struct {
	Data       []json.RawMessage `json:"data"`
	Pagination pagination        `json:"pagination"`
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(body)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// handleHealth reports liveness.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleListFiles returns ingest metadata for all known capture files.
func (s *Server) handleListFiles(w http.ResponseWriter, r *http.Request) {
	files, err := s.store.ListFiles(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if files == nil {
		files = []store.FileInfo{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"files": files})
}

// handleReload re-scans the data directory and ingests new/changed files.
func (s *Server) handleReload(w http.ResponseWriter, r *http.Request) {
	res, err := s.loader.Sync(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// handleFlows applies query filters and returns a paginated page of records.
func (s *Server) handleFlows(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	page, err := parsePositiveInt(q.Get("page"), 1)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid page: "+err.Error())
		return
	}
	pageSize, err := parsePositiveInt(q.Get("page_size"), defaultPageSize)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid page_size: "+err.Error())
		return
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}

	fq := store.FlowQuery{
		File:     q.Get("file"),
		SrcAddr:  q.Get("src_addr"),
		DstAddr:  q.Get("dst_addr"),
		Proto:    q.Get("proto"),
		Type:     q.Get("type"),
		Search:   q.Get("q"),
		Page:     page,
		PageSize: pageSize,
	}

	if fq.SrcPort, err = parseOptionalInt(q.Get("src_port")); err != nil {
		writeError(w, http.StatusBadRequest, "invalid src_port: "+err.Error())
		return
	}
	if fq.DstPort, err = parseOptionalInt(q.Get("dst_port")); err != nil {
		writeError(w, http.StatusBadRequest, "invalid dst_port: "+err.Error())
		return
	}
	if fq.BytesMin, err = parseOptionalInt(q.Get("bytes_min")); err != nil {
		writeError(w, http.StatusBadRequest, "invalid bytes_min: "+err.Error())
		return
	}
	if fq.BytesMax, err = parseOptionalInt(q.Get("bytes_max")); err != nil {
		writeError(w, http.StatusBadRequest, "invalid bytes_max: "+err.Error())
		return
	}

	total, flows, err := s.store.QueryFlows(r.Context(), fq)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	totalPages := int64(0)
	if pageSize > 0 {
		totalPages = (total + int64(pageSize) - 1) / int64(pageSize)
	}

	writeJSON(w, http.StatusOK, flowsResponse{
		Data: flows,
		Pagination: pagination{
			Page:       page,
			PageSize:   pageSize,
			Total:      total,
			TotalPages: totalPages,
		},
	})
}

// parsePositiveInt parses a 1+ integer, returning def for empty input.
func parsePositiveInt(s string, def int) (int, error) {
	if s == "" {
		return def, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, err
	}
	if n < 1 {
		return 0, errPositive
	}
	return n, nil
}

// parseOptionalInt returns a pointer to the parsed int64, or nil for empty input.
func parseOptionalInt(s string) (*int64, error) {
	if s == "" {
		return nil, nil
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return nil, err
	}
	return &n, nil
}

type constError string

func (e constError) Error() string { return string(e) }

const errPositive constError = "must be >= 1"
