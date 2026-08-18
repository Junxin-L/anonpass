# Threat Model

Anonpass separates identity from redemption.

The issuer sees the account and a blinded token. The gateway sees the final token and signature. The gateway should not learn the account from the redemption request.

## Assets

- issuer RSA private key
- issuer public key set and key ids
- per-account quota state
- spent-token state
- token signatures
- account identifiers at issuance time

## Trust Boundaries

```text
account -> issuer -> blind signature -> client -> gateway -> receipt
```

The issuer boundary handles authentication and quota. The gateway boundary handles anonymous redemption and replay protection.

No normal gateway request should contain an account name.

## Adversaries

Malicious client:

- tries to mint extra tokens
- replays an already redeemed token
- submits malformed hex or oversized JSON
- races many redemptions of the same token

Curious gateway:

- sees token, signature, key id, and timing
- must not see account identity

Curious issuer:

- sees account and blinded token
- must not recognize the final redemption token

Compromised storage:

- may reveal spent token hashes and quota counters
- should not reveal raw token values or private keys

## Required Properties

Replay protection:

Only one redemption can succeed for a token. In PostgreSQL this is enforced by `token_hash` as a primary key and an atomic insert.

Quota protection:

Issuer replicas must not over-issue for the same account and window. In PostgreSQL this is enforced by an atomic upsert on `(account, window)`.

Key expiry:

Gateways reject tokens signed under a key after `not_after`.

Gateway privacy:

Gateway APIs accept token material and key ids, not account names.

## Known Limits

Anonpass does not hide timing correlation by itself. If a user issues a token and immediately redeems it, external logs may still correlate the events.

Local PEM keys are suitable for development and single-host demos. Production deployments should use KMS or HSM-backed signing.

The browser console uses demo endpoints that simulate client-side blind/unblind steps on the server. Those endpoints are for demos and should not be exposed as a privacy boundary in production.
