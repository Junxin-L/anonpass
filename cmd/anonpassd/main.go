package main

import (
	"flag"
	"log"
	"net/http"
	"time"

	"anonpass/internal/httpapi"
	"anonpass/internal/keyfile"
	"anonpass/internal/tokens"
	"anonpass/internal/webui"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	keyID := flag.String("key-id", "local-1", "issuer key id")
	keyPath := flag.String("key-file", "data/issuer.pem", "RSA private key path")
	replayPath := flag.String("replay-db", "data/replay.db", "spent-token database path")
	replayPostgresDSN := flag.String("replay-postgres-dsn", "", "PostgreSQL DSN for shared replay storage")
	quotaPostgresDSN := flag.String("quota-postgres-dsn", "", "PostgreSQL DSN for shared issuer quota storage")
	quota := flag.Int("quota", 5, "tokens per account")
	tokenTTL := flag.Duration("token-ttl", 24*time.Hour, "maximum token lifetime for this issuer key")
	flag.Parse()

	key, err := keyfile.LoadOrCreate(*keyPath, 2048)
	if err != nil {
		log.Fatalf("load issuer key: %v", err)
	}
	quotaStore, err := openQuotaStore(*quotaPostgresDSN, *replayPostgresDSN)
	if err != nil {
		log.Fatalf("open quota store: %v", err)
	}
	defer quotaStore.Close()

	issuer := tokens.NewIssuerWithStore(*keyID, key, *quota, time.Now().Add(*tokenTTL).Unix(), quotaStore)

	replayStore, err := openReplayStore(*replayPostgresDSN, *replayPath)
	if err != nil {
		log.Fatalf("open replay store: %v", err)
	}
	defer replayStore.Close()

	gateway := tokens.NewGatewayWithStore(replayStore, issuer.PublicKey())
	api := httpapi.New(issuer, gateway)
	mux := http.NewServeMux()
	mux.Handle("/v1/", api)
	mux.Handle("/", webui.Handler())

	server := &http.Server{
		Addr:              *addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	log.Printf("anonpass listening on %s with key_id=%s", *addr, *keyID)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server stopped: %v", err)
	}
}

func openReplayStore(postgresDSN, boltPath string) (tokens.ReplayStore, error) {
	if postgresDSN != "" {
		return tokens.OpenPostgresReplayStore(postgresDSN)
	}
	return tokens.OpenBoltReplayStore(boltPath)
}

func openQuotaStore(quotaPostgresDSN, replayPostgresDSN string) (tokens.QuotaStore, error) {
	if quotaPostgresDSN != "" {
		return tokens.OpenPostgresQuotaStore(quotaPostgresDSN)
	}
	if replayPostgresDSN != "" {
		return tokens.OpenPostgresQuotaStore(replayPostgresDSN)
	}
	return tokens.NewMemoryQuotaStore(), nil
}
