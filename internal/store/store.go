// Package store wraps a SQLite database holding ingested goflow2 flow records
// and exposes filtered, paginated queries.
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
)

// Store is a handle to the flows database.
type Store struct {
	db *sql.DB
}

// schema creates the tables and indexes used for ingestion and querying.
const schema = `
CREATE TABLE IF NOT EXISTS flows (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    file             TEXT    NOT NULL,
    type             TEXT,
    src_addr         TEXT,
    dst_addr         TEXT,
    proto            TEXT,
    src_port         INTEGER,
    dst_port         INTEGER,
    bytes            INTEGER,
    packets          INTEGER,
    time_received_ns INTEGER,
    raw_json         TEXT    NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_flows_file      ON flows(file);
CREATE INDEX IF NOT EXISTS idx_flows_src_addr  ON flows(src_addr);
CREATE INDEX IF NOT EXISTS idx_flows_dst_addr  ON flows(dst_addr);
CREATE INDEX IF NOT EXISTS idx_flows_proto     ON flows(proto);
CREATE INDEX IF NOT EXISTS idx_flows_src_port  ON flows(src_port);
CREATE INDEX IF NOT EXISTS idx_flows_dst_port  ON flows(dst_port);
CREATE INDEX IF NOT EXISTS idx_flows_bytes     ON flows(bytes);
CREATE INDEX IF NOT EXISTS idx_flows_type      ON flows(type);
CREATE INDEX IF NOT EXISTS idx_flows_time      ON flows(time_received_ns);

CREATE TABLE IF NOT EXISTS ingest_files (
    path      TEXT PRIMARY KEY,
    mtime_ns  INTEGER NOT NULL,
    size      INTEGER NOT NULL,
    row_count INTEGER NOT NULL
);
`

// Open opens (or creates) the SQLite database at dsn and applies the schema.
// Use ":memory:" for an ephemeral store, or a file path for persistence.
func Open(dsn string) (*Store, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// Pragmas that meaningfully speed up bulk ingest of large captures.
	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL;",
		"PRAGMA synchronous=NORMAL;",
		"PRAGMA temp_store=MEMORY;",
	} {
		if _, err := db.Exec(pragma); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("pragma %q: %w", pragma, err)
		}
	}
	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	return &Store{db: db}, nil
}

// Close releases the underlying database handle.
func (s *Store) Close() error { return s.db.Close() }

// DB exposes the raw handle for the ingest package's batched transactions.
func (s *Store) DB() *sql.DB { return s.db }

// FileInfo records the ingest bookkeeping for a single source file.
type FileInfo struct {
	Path     string `json:"file"`
	MtimeNs  int64  `json:"mtime_ns"`
	Size     int64  `json:"size"`
	RowCount int64  `json:"row_count"`
}

// IngestedFile returns the recorded ingest metadata for path, if present.
func (s *Store) IngestedFile(ctx context.Context, path string) (FileInfo, bool, error) {
	var fi FileInfo
	row := s.db.QueryRowContext(ctx,
		`SELECT path, mtime_ns, size, row_count FROM ingest_files WHERE path = ?`, path)
	err := row.Scan(&fi.Path, &fi.MtimeNs, &fi.Size, &fi.RowCount)
	if err == sql.ErrNoRows {
		return FileInfo{}, false, nil
	}
	if err != nil {
		return FileInfo{}, false, err
	}
	return fi, true, nil
}

// ListFiles returns ingest metadata for all known files, ordered by name.
func (s *Store) ListFiles(ctx context.Context) ([]FileInfo, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT path, mtime_ns, size, row_count FROM ingest_files ORDER BY path`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []FileInfo
	for rows.Next() {
		var fi FileInfo
		if err := rows.Scan(&fi.Path, &fi.MtimeNs, &fi.Size, &fi.RowCount); err != nil {
			return nil, err
		}
		out = append(out, fi)
	}
	return out, rows.Err()
}

// DeleteFile removes a file's rows and its ingest record (used before re-ingest).
func (s *Store) DeleteFile(ctx context.Context, path string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM flows WHERE file = ?`, path); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM ingest_files WHERE path = ?`, path); err != nil {
		return err
	}
	return tx.Commit()
}

// RecordFile upserts the ingest bookkeeping row for a completed file.
func (s *Store) RecordFile(ctx context.Context, fi FileInfo) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO ingest_files (path, mtime_ns, size, row_count)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(path) DO UPDATE SET
			mtime_ns  = excluded.mtime_ns,
			size      = excluded.size,
			row_count = excluded.row_count`,
		fi.Path, fi.MtimeNs, fi.Size, fi.RowCount)
	return err
}

// FlowQuery captures the supported filters for the flows endpoint. Zero-valued
// (nil) fields are omitted from the WHERE clause.
type FlowQuery struct {
	File     string
	SrcAddr  string
	DstAddr  string
	Proto    string
	Type     string
	SrcPort  *int64
	DstPort  *int64
	BytesMin *int64
	BytesMax *int64
	Search   string // substring match across src_addr, dst_addr, proto

	Page     int
	PageSize int
}

// buildWhere assembles the WHERE clause and its arguments from a FlowQuery.
func (q FlowQuery) buildWhere() (string, []any) {
	var clauses []string
	var args []any

	add := func(cond string, val any) {
		clauses = append(clauses, cond)
		args = append(args, val)
	}

	if q.File != "" {
		add("file = ?", q.File)
	}
	if q.SrcAddr != "" {
		add("src_addr = ?", q.SrcAddr)
	}
	if q.DstAddr != "" {
		add("dst_addr = ?", q.DstAddr)
	}
	if q.Proto != "" {
		add("proto = ?", q.Proto)
	}
	if q.Type != "" {
		add("type = ?", q.Type)
	}
	if q.SrcPort != nil {
		add("src_port = ?", *q.SrcPort)
	}
	if q.DstPort != nil {
		add("dst_port = ?", *q.DstPort)
	}
	if q.BytesMin != nil {
		add("bytes >= ?", *q.BytesMin)
	}
	if q.BytesMax != nil {
		add("bytes <= ?", *q.BytesMax)
	}
	if q.Search != "" {
		like := "%" + q.Search + "%"
		clauses = append(clauses, "(src_addr LIKE ? OR dst_addr LIKE ? OR proto LIKE ?)")
		args = append(args, like, like, like)
	}

	if len(clauses) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

// QueryFlows returns the total number of matching rows and the requested page
// of records as raw JSON messages (the original goflow2 objects).
func (s *Store) QueryFlows(ctx context.Context, q FlowQuery) (total int64, flows []json.RawMessage, err error) {
	where, args := q.buildWhere()

	if err = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM flows"+where, args...).Scan(&total); err != nil {
		return 0, nil, fmt.Errorf("count flows: %w", err)
	}

	offset := (q.Page - 1) * q.PageSize
	pageArgs := append(append([]any{}, args...), q.PageSize, offset)

	rows, err := s.db.QueryContext(ctx,
		"SELECT raw_json FROM flows"+where+" ORDER BY id LIMIT ? OFFSET ?", pageArgs...)
	if err != nil {
		return 0, nil, fmt.Errorf("select flows: %w", err)
	}
	defer func() { _ = rows.Close() }()

	flows = make([]json.RawMessage, 0, q.PageSize)
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return 0, nil, err
		}
		flows = append(flows, json.RawMessage(raw))
	}
	return total, flows, rows.Err()
}
