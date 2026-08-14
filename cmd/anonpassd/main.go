package main

import (
	"flag"
	"log"
	"net/http"
	"time"

	"anonpass/internal/httpapi"
	"anonpass/internal/tokens"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	quota := flag.Int("quota", 5, "tokens per account")
	flag.Parse()

	issuer, err := tokens.NewIssuer("local-1", 2048, *quota)
	if err != nil {
		log.Fatalf("create issuer: %v", err)
	}
	gateway := tokens.NewGateway(issuer.PublicKey())

	server := &http.Server{
		Addr:              *addr,
		Handler:           httpapi.New(issuer, gateway),
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("anonpass listening on %s", *addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server stopped: %v", err)
	}
}
