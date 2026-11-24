// Command bff serves the jobapps GraphQL API at /graphql.
package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/graph-gophers/graphql-go/relay"
	_ "github.com/lib/pq"

	"jobapps/bff/graph"
	"jobapps/bff/plugins"
	_ "jobapps/bff/plugins/all" // registers every company plugin
	"jobapps/bff/scan"
	"jobapps/bff/store"
)

func main() {
	dbURL := env("DATABASE_URL", "postgres://jobapps:jobapps@localhost:5432/jobapps?sslmode=disable")
	addr := env("ADDR", ":8080")

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		log.Fatalf("connecting to database: %v", err)
	}

	st := store.New(db)
	concurrency, _ := strconv.Atoi(os.Getenv("SCAN_CONCURRENCY"))
	ps := plugins.All()
	log.Printf("%d scan plugins registered", len(ps))
	scans := scan.NewManager(ps, st, scan.Options{Concurrency: concurrency})
	schema, err := graph.NewSchema(st, scans)
	if err != nil {
		log.Fatalf("parsing schema: %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("POST /graphql", &relay.Handler{Schema: schema})
	mux.Handle("POST /graphql/stream", graph.StreamHandler(schema))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := db.PingContext(r.Context()); err != nil {
			http.Error(w, "database unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte("ok"))
	})

	srv := &http.Server{Addr: addr, Handler: cors(mux), ReadHeaderTimeout: 5 * time.Second}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()

	log.Printf("listening on %s", addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

// cors allows any origin. Fine because this only ever runs locally.
func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
