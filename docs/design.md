# Design Notes

Anonpass is built around one separation: issuing a token is not the same event as spending it.

The issuer is allowed to know the account. It decides whether the account has quota left. The gateway is allowed to know that a token is valid. It should not need to know who received that token.

This is the reason for using blind signatures. The client blinds a token before the issuer signs it. After unblinding, the client has a signature that the gateway can verify. The issuer signed the token, but it cannot recognize the final token value at redemption time.

## Services

The local server contains both roles because this is a compact project, but the code keeps them separate.

The issuer owns:

- the signing key
- the active key id
- the key expiry time
- per-account quota by window

The gateway owns:

- the issuer public-key set
- the durable spent-token table
- redemption receipts

The client owns:

- token randomness
- blind-signature state
- the unblinded signature

Keeping these roles separate makes the privacy boundary easy to inspect in code. The issuer API receives an account and a blinded token. The gateway API receives a token and a signature. There is no endpoint where both account and final token are required.

The browser console uses `/v1/demo/*` endpoints to show the protocol steps without shipping a JavaScript implementation of CIRCL. Those endpoints are for inspection and demos. The normal issuer and gateway APIs remain separate.

## Cryptography

The signature implementation comes from Cloudflare CIRCL:

```text
github.com/cloudflare/circl/blindsign/blindrsa
```

The selected scheme is RFC 9474 RSABSSA with randomized SHA-384/PSS. The wrapper in `internal/blindrsa` is intentionally small. It does not reimplement the signature math; it only adapts CIRCL's client, signer, and verifier to the rest of the service.

The randomized variant is used so the prepared message includes fresh randomness before blinding. The token that reaches the gateway is this prepared message. It is still an anonymous random value, but it matches the message that CIRCL verifies.

## Request Flow

Issuance:

```text
client -> issuer: account, blinded_token
issuer -> client: key_id, blind_signature
```

Redemption:

```text
client -> gateway: key_id, token, signature
gateway -> client: receipt
```

During issuance, the issuer checks quota before signing. During redemption, the gateway verifies the signature first and then inserts the token hash into the spent table.

The order matters. A bad signature should not consume storage. A valid token must be recorded atomically before the gateway accepts it. The gateway also rejects requests for keys whose `not_after` time has passed.

## Replay State

The server has two durable replay options.

For one process, bbolt stores spent tokens on disk. Each spent token is stored by `SHA256(token)`, not by the raw token bytes.

For multiple gateway processes, PostgreSQL gives a shared atomic replay check. The table has `token_hash` as the primary key. A redemption succeeds only when this insert creates a new row:

```sql
INSERT INTO spent_tokens(token_hash, receipt_json, redeemed_at)
VALUES ($1, $2, $3)
ON CONFLICT DO NOTHING;
```

The implementation stores the full receipt as JSONB as well, so a replayed token can return the original receipt. Unit tests use the in-memory store when persistence is not part of the test.

The schema is also kept in `migrations/001_create_spent_tokens.sql`.
You can apply the whole migration set with `go run ./cmd/anonpassmigrate`.

## Quota State

Many users mainly stress the issuer. If two issuer replicas receive requests for the same account at the same time, quota must be updated atomically. Otherwise the service can over-issue.

For local tests, the issuer uses an in-memory quota store. For multi-user deployment, PostgreSQL stores one row per `(account, window)`:

```sql
CREATE TABLE quota_windows (
  account TEXT NOT NULL,
  window TEXT NOT NULL,
  used_count INTEGER NOT NULL,
  quota_limit INTEGER NOT NULL,
  updated_at BIGINT NOT NULL,
  PRIMARY KEY (account, window)
);
```

Issuance uses one upsert statement. It increments `used_count` only while the current count is below the limit. If no row is returned, the account has no quota left for that window.

The current window is a UTC date such as `2026-08-14`. That keeps the policy simple and makes quota reset behavior easy to inspect.

The schema is also kept in `migrations/002_create_quota_windows.sql`.
The same migration command applies it in order.

## Many Clients

With thousands of clients, the first goal is correctness under concurrency. The issuer must not over-issue when many requests hit the same account, and the gateway must not accept the same token twice when many gateway replicas are running.

Anonpass handles those two points with shared atomic stores:

- quota: PostgreSQL upsert on `(account, window)`
- replay: PostgreSQL insert on `token_hash`

The local HTTP server also sets read, write, idle, and header limits so slow clients cannot hold connections forever.

The repository includes `cmd/anonpassload` for stress runs. It can run a fixed flow such as "10,000 clients each issue two tokens", or random traffic such as "50,000 requests across 10,000 clients with 60% issue and 5% replay". This is not a full benchmark suite, but it is enough to expose quota bugs, replay bugs, and obvious concurrency bottlenecks.

The expensive operation is still blind RSA signing. A high-throughput deployment should run multiple issuer replicas, put quota in PostgreSQL, and usually issue several tokens per authenticated session so users do not need a fresh signature for every gateway request.

## Key Rotation

Every blind signature response includes a `key_id`. The gateway keeps a map from `key_id` to issuer public key and its `not_after` time.

The local server loads or creates the issuer RSA key from a PEM file. That avoids losing the signing key on restart. New issuance uses the active key. Gateways can keep old public keys until their `not_after` time passes, and revoked keys can be removed immediately.

This is why the key id is part of the protocol messages even though the demo starts with one key.

## What The System Protects

The gateway cannot learn which account received a token from the normal redemption request. The issuer cannot recognize the final token it signed. A client cannot spend the same token twice once the gateway has recorded the token hash. A client also cannot mint extra valid tokens without getting signatures from the issuer.

The system does not hide timing by itself. If Alice requests a token and immediately uses it, logs from both services may still correlate the events. This is a metadata problem, not a signature problem.

## Remaining Production Work

Signing keys should eventually live in KMS or an HSM instead of a local PEM file. Logs should avoid raw token values and should be checked for timing leaks. For very large deployments, quota and replay tables would need retention jobs and partitioning by date.

The main design would stay the same: identity at the issuer, anonymous redemption at the gateway, and a one-time replay check in the middle.
