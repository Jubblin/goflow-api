// Command server starts the goflow2 REST API: it ingests goflow2*.json files
// from a data directory into SQLite and serves filtered, paginated JSON.
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"time"

	"fmt"

	"github.com/richardw/goflow-api/internal/api"
	"github.com/richardw/goflow-api/internal/ingest"
	"github.com/richardw/goflow-api/internal/store"
	"github.com/richardw/goflow-api/internal/version"
)

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	dataDir := flag.String("data-dir", envOr("DATA_DIR", "./data"), "directory containing goflow2*.json files")
	listenAddr := flag.String("listen", envOr("LISTEN_ADDR", ":8080"), "HTTP listen address")
	dbPath := flag.String("db", envOr("DB_PATH", "./goflow.db"), "SQLite database path (use :memory: for ephemeral)")
	flag.Parse()

	if *showVersion {
		fmt.Printf("goflow-api %s (commit %s, built %s)\n", version.Version, version.Commit, version.Date)
		return
	}

	st, err := store.Open(*dbPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	loader := ingest.New(*dataDir, st)

	log.Printf("scanning %s for goflow2*.json ...", *dataDir)
	res, err := loader.Sync(context.Background())
	if err != nil {
		log.Fatalf("initial ingest: %v", err)
	}
	log.Printf("ingest complete: %d discovered, %d ingested, %d skipped, %d total rows",
		len(res.Discovered), len(res.Ingested), len(res.Skipped), res.TotalRows)

	srv := api.NewServer(st, loader)
	httpSrv := &http.Server{
		Addr:              *listenAddr,
		Handler:           srv.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	log.Printf("listening on %s", *listenAddr)
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server: %v", err)
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
