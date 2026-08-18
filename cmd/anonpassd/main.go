package main

import (
	"crypto/rsa"
	"flag"
	"log"
	"net/http"
	"strings"
	"time"

	"anonpass/internal/httpapi"
	"anonpass/internal/keyfile"
	"anonpass/internal/observability"
	"anonpass/internal/tokens"
	"anonpass/internal/webui"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	role := flag.String("role", "all", "service role: all, issuer, or gateway")
	keyID := flag.String("key-id", "local-1", "issuer key id")
	keyPath := flag.String("key-file", "data/issuer.pem", "RSA private key path")
	publicKeyPath := flag.String("public-key-file", "data/issuer.pem.pub", "RSA public key path for gateway role")
	replayPath := flag.String("replay-db", "data/replay.db", "spent-token database path")
	replayPostgresDSN := flag.String("replay-postgres-dsn", "", "PostgreSQL DSN for shared replay storage")
	quotaPostgresDSN := flag.String("quota-postgres-dsn", "", "PostgreSQL DSN for shared issuer quota storage")
	quota := flag.Int("quota", 5, "tokens per account")
	tokenTTL := flag.Duration("token-ttl", 24*time.Hour, "maximum token lifetime for this issuer key")
	maxBodyBytes := flag.Int64("max-body-bytes", 1<<20, "maximum request body size in bytes")
	rateLimitRPS := flag.Float64("rate-limit-rps", 0, "per-client IP request rate limit in requests per second")
	rateLimitBurst := flag.Float64("rate-limit-burst", 20, "per-client IP burst size")
	flag.Parse()

	enableIssuer, enableGateway, enableDemo := roleFlags(*role)

	var issuer *tokens.Issuer
	var issuerKey tokens.PublicKey
	notAfter := time.Now().Add(*tokenTTL).Unix()
	if enableIssuer || enableDemo || enableGateway {
		if enableIssuer || enableDemo {
			key, err := keyfile.LoadOrCreate(*keyPath, 2048)
			if err != nil {
				log.Fatalf("load issuer key: %v", err)
			}
			quotaStore, err := openQuotaStore(*quotaPostgresDSN, *replayPostgresDSN)
			if err != nil {
				log.Fatalf("open quota store: %v", err)
			}
			defer quotaStore.Close()
			issuer = tokens.NewIssuerWithStore(*keyID, key, *quota, notAfter, quotaStore)
			issuerKey = issuer.PublicKey()
		} else {
			pub, err := loadGatewayPublicKey(*publicKeyPath, *keyPath)
			if err != nil {
				log.Fatalf("load gateway public key: %v", err)
			}
			issuerKey = tokens.PublicKey{
				KeyID:    *keyID,
				Key:      pub,
				N:        pub.N.Text(16),
				E:        pub.E,
				NotAfter: notAfter,
			}
		}
	}

	var gateway *tokens.Gateway
	if enableGateway || enableDemo {
		replayStore, err := openReplayStore(*replayPostgresDSN, *replayPath)
		if err != nil {
			log.Fatalf("open replay store: %v", err)
		}
		defer replayStore.Close()
		gateway = tokens.NewGatewayWithStore(replayStore, issuerKey)
	}

	api := httpapi.NewWithOptions(issuer, gateway, httpapi.Options{
		EnableIssuer:  enableIssuer || enableDemo,
		EnableGateway: enableGateway || enableDemo,
		EnableDemo:    enableDemo,
	})
	mux := http.NewServeMux()
	metrics := observability.NewMetrics()
	mux.Handle("/v1/", api)
	mux.Handle("/metrics", metrics.Handler())
	if enableDemo {
		mux.Handle("/", webui.Handler())
	} else {
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			http.NotFound(w, r)
		})
	}

	handler := observability.BodyLimit{Bytes: *maxBodyBytes}.Middleware(mux)
	handler = metrics.Middleware(handler)
	handler = observability.NewRateLimiter(*rateLimitRPS, *rateLimitBurst).Middleware(handler)

	server := &http.Server{
		Addr:              *addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	log.Printf("anonpass listening on %s with key_id=%s role=%s", *addr, *keyID, *role)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server stopped: %v", err)
	}
}

func roleFlags(role string) (bool, bool, bool) {
	switch strings.ToLower(role) {
	case "issuer":
		return true, false, false
	case "gateway":
		return false, true, false
	default:
		return true, true, true
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

func loadGatewayPublicKey(publicPath, privatePath string) (*rsa.PublicKey, error) {
	pub, err := keyfile.LoadPublic(publicPath)
	if err == nil {
		return pub, nil
	}
	key, err := keyfile.LoadOrCreate(privatePath, 2048)
	if err != nil {
		return nil, err
	}
	return &key.PublicKey, nil
}
