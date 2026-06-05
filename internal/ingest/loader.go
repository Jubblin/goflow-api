// Package ingest discovers goflow2*.json captures and loads their NDJSON
// records into the SQLite store.
package ingest

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"

	"github.com/richardw/goflow-api/internal/model"
	"github.com/richardw/goflow-api/internal/store"
)

// batchSize is the number of rows inserted per transaction during ingest.
const batchSize = 1000

// maxLineBytes is the scanner buffer ceiling for a single NDJSON record.
const maxLineBytes = 4 * 1024 * 1024

// Loader ingests files from a data directory into a store.
type Loader struct {
	dataDir string
	store   *store.Store
}

// New builds a Loader for the given data directory and store.
func New(dataDir string, s *store.Store) *Loader {
	return &Loader{dataDir: dataDir, store: s}
}

// Result summarises one Sync pass.
type Result struct {
	Discovered []string `json:"discovered"`
	Ingested   []string `json:"ingested"`
	Skipped    []string `json:"skipped"`
	TotalRows  int64    `json:"total_rows"`
}

// Discover returns absolute paths of files matching goflow2*.json in dataDir.
func (l *Loader) Discover() ([]string, error) {
	matches, err := filepath.Glob(filepath.Join(l.dataDir, "goflow2*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob data dir: %w", err)
	}
	sort.Strings(matches)
	return matches, nil
}

// Sync discovers files and ingests any that are new or changed since last seen.
// Files whose mtime and size match the recorded ingest are skipped.
func (l *Loader) Sync(ctx context.Context) (Result, error) {
	var res Result

	matches, err := l.Discover()
	if err != nil {
		return res, err
	}

	for _, path := range matches {
		name := filepath.Base(path)
		res.Discovered = append(res.Discovered, name)

		info, err := os.Stat(path)
		if err != nil {
			return res, fmt.Errorf("stat %s: %w", path, err)
		}
		mtimeNs := info.ModTime().UnixNano()
		size := info.Size()

		prev, found, err := l.store.IngestedFile(ctx, name)
		if err != nil {
			return res, err
		}
		if found && prev.MtimeNs == mtimeNs && prev.Size == size {
			res.Skipped = append(res.Skipped, name)
			res.TotalRows += prev.RowCount
			continue
		}

		// Changed or new: drop any existing rows then re-ingest.
		if found {
			if err := l.store.DeleteFile(ctx, name); err != nil {
				return res, fmt.Errorf("clear stale rows for %s: %w", name, err)
			}
		}

		log.Printf("ingesting %s (%d bytes)...", name, size)
		rows, err := l.ingestFile(ctx, path, name)
		if err != nil {
			return res, fmt.Errorf("ingest %s: %w", name, err)
		}
		if err := l.store.RecordFile(ctx, store.FileInfo{
			Path: name, MtimeNs: mtimeNs, Size: size, RowCount: rows,
		}); err != nil {
			return res, err
		}
		log.Printf("ingested %s: %d rows", name, rows)
		res.Ingested = append(res.Ingested, name)
		res.TotalRows += rows
	}

	return res, nil
}

// ingestFile streams one NDJSON file into the flows table in batches.
func (l *Loader) ingestFile(ctx context.Context, path, name string) (int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineBytes)

	db := l.store.DB()
	var total int64
	var lineNo int

	for {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return total, err
		}
		stmt, err := tx.PrepareContext(ctx, `
			INSERT INTO flows
				(file, type, src_addr, dst_addr, proto, src_port, dst_port, bytes, packets, time_received_ns, raw_json)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
		if err != nil {
			tx.Rollback()
			return total, err
		}

		var inBatch int
		eof := false
		for inBatch < batchSize {
			if !scanner.Scan() {
				eof = true
				break
			}
			lineNo++
			line := scanner.Bytes()
			if len(line) == 0 {
				continue
			}

			var fl model.Flow
			if err := json.Unmarshal(line, &fl); err != nil {
				// Skip malformed lines but keep going; log for visibility.
				log.Printf("%s:%d: skipping malformed JSON: %v", name, lineNo, err)
				continue
			}

			// Persist a copy of the original line as the canonical raw record.
			raw := make([]byte, len(line))
			copy(raw, line)

			if _, err := stmt.ExecContext(ctx, name, fl.Type, fl.SrcAddr, fl.DstAddr,
				fl.Proto, fl.SrcPort, fl.DstPort, fl.Bytes, fl.Packets, fl.TimeReceivedNs, string(raw)); err != nil {
				stmt.Close()
				tx.Rollback()
				return total, err
			}
			inBatch++
			total++
		}

		stmt.Close()
		if err := tx.Commit(); err != nil {
			return total, err
		}
		if eof {
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return total, fmt.Errorf("read %s: %w", name, err)
	}
	return total, nil
}
