# Anonpass

Anonpass is a Go service for anonymous, one-time access tokens. It supports many clients by keeping quota and replay checks in atomic stores instead of in process-local request state.

The problem is: A user signs in and has a quota. Later, the user wants to access a gateway. The gateway should be able to check that the request is allowed, and it should reject the same token if it appears again. At the same time, the gateway should not learn which account received the token.

Anonpass separates those two moments. The issuer handles identity and quota. The gateway handles redemption and replay. The client connects them with a blind signature.

The protocol is short:

1. The client creates a random token.
2. The client blinds the token and sends the blinded value to the issuer.
3. The issuer checks the account quota and signs the blinded value.
4. The client unblinds the signature.
5. The gateway verifies the token and records it as spent.

The issuer never sees the final token. The gateway never sees the account.

The current implementation supports multi-client use in three ways:

- each account has its own quota window
- issuer replicas can share quota through PostgreSQL
- gateway replicas can share replay state through PostgreSQL

## Why The Split Matters

A bearer token or JWT is faster and easier to deploy. It is also more linkable. In many systems that is acceptable, but it is the wrong tradeoff when the spend path should not carry account identity.

Anonpass keeps the useful part of an access-token system: quota, verification, and replay protection. It removes the direct link between issuance and redemption.

The cryptographic part uses Cloudflare CIRCL's implementation of RFC 9474 RSA Blind Signatures, using the randomized SHA-384/PSS variant. The project does not implement its own signature scheme. The local code is about the service boundary: issuer state, gateway state, key ids, HTTP handlers, and tests.

## Performance

Benchmarks were run on an Apple M1 Pro with 2048-bit RSA keys.

```text
BenchmarkBlindSignUnblindVerify-8      about 3.66 ms/op    56.2 KB/op   178 allocs/op
BenchmarkIssueAndRedeem-8              about 3.83 ms/op    57.7 KB/op   183 allocs/op
BenchmarkIssueAndRedeemBolt-8          about 12.2 ms/op    87.4 KB/op   236 allocs/op
```

That is roughly 273 blind-signature flows per second for the crypto path, 261 full issue-and-redeem flows per second with the memory store, and 82 full flows per second with durable bbolt replay storage.

This should not be compared to JWT on speed alone. JWT wins that benchmark easily. The comparison here is about what the gateway learns. Anonpass spends a few milliseconds so the gateway can validate a one-time token without receiving the account identity.

In a larger deployment, the bottleneck would likely move to durable replay storage, network calls, and cross-region consistency.

## Run

Start the server:

```sh
go run ./cmd/anonpassd \
  -addr :8080 \
  -quota 5 \
  -key-file data/issuer.pem \
  -replay-db data/replay.db \
  -replay-postgres-dsn "" \
  -quota-postgres-dsn "" \
  -token-ttl 24h
```

The server keeps the RSA issuer key in `data/issuer.pem` and records spent tokens in `data/replay.db`. If the process restarts, old tokens cannot be spent again.

Then open:

```text
http://localhost:8080
```

The console lets you issue a token, redeem it, and try the replay path from the browser.

For multiple gateway processes, use PostgreSQL instead of the local bbolt file:

```sh
go run ./cmd/anonpassd \
  -addr :8080 \
  -quota 5 \
  -key-file data/issuer.pem \
  -replay-postgres-dsn 'postgres://user:pass@host:5432/anonpass?sslmode=require' \
  -quota-postgres-dsn 'postgres://user:pass@host:5432/anonpass?sslmode=require'
```

The PostgreSQL replay store uses `token_hash` as a primary key and accepts a redemption only when `INSERT ... ON CONFLICT DO NOTHING` inserts a new row.

The PostgreSQL quota store uses `(account, window)` as a primary key and increments `used_count` in one statement. Many issuer replicas can receive requests for the same account without issuing more than the configured quota.

Run the local protocol demo:

```sh
go run ./cmd/anonpassdemo
```

Run tests and benchmarks:

```sh
GOCACHE=/tmp/anonpass-gocache go test ./...
GOCACHE=/tmp/anonpass-gocache go test -bench=. -benchmem ./internal/blindrsa ./internal/tokens
```

`GOCACHE` is only needed in restricted environments where Go cannot write to the default cache directory.

Run a multi-client load test against a running server:

```sh
go run ./cmd/anonpassload \
  -url http://127.0.0.1:8080 \
  -clients 10000 \
  -tokens 2 \
  -concurrency 200
```

The load tool creates many client accounts, issues tokens, redeems them, and verifies that replay attempts are rejected.

You can also run random traffic:

```sh
go run ./cmd/anonpassload \
  -url http://127.0.0.1:8080 \
  -clients 10000 \
  -requests 50000 \
  -concurrency 300 \
  -issue-rate 0.60 \
  -replay-rate 0.05
```

In random mode, each request picks a random client and then chooses issue, redeem, or replay according to the rates.

## API

Fetch the issuer key:

```sh
curl -s localhost:8080/v1/issuer/key
```

Ask the issuer to sign a blinded token:

```sh
curl -s -X POST localhost:8080/v1/issuer/blind-sign \
  -H 'content-type: application/json' \
  -d '{"account":"alice","blinded_token":"hex"}'
```

Redeem a token at the gateway:

```sh
curl -s -X POST localhost:8080/v1/gateway/redeem \
  -H 'content-type: application/json' \
  -d '{"key_id":"local-1","token":"hex","signature":"hex"}'
```

The curl examples show the server boundary. The client-side blind and unblind flow is in `cmd/anonpassdemo` and the unit tests.

## Layout

```text
cmd/anonpassd         HTTP server
cmd/anonpassdemo      local end-to-end demo
cmd/anonpassload      multi-client load test
internal/blindrsa     wrapper around CIRCL RFC 9474 RSABSSA
internal/keyfile      PEM key loading and creation
internal/tokens       issuer, gateway, quota stores, replay stores
internal/httpapi      JSON handlers
internal/webui        embedded browser console
docs/design.md        design notes
docs/api.md           API reference
```

## Operational Notes

The current server has durable replay protection, shared PostgreSQL replay checks, PostgreSQL-backed multi-user quota, file-backed key loading, key expiry through `not_after`, and basic HTTP timeouts. Single-node deployments can still use bbolt for replay state and memory quota for local testing.

## AI Use

AI was used to help edit the wording of the documentation. The protocol choice, code structure, tests, benchmark runs, and final review remain the author's responsibility.
