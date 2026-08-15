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
- per-account quota

The gateway owns:

- the issuer public-key set
- the durable spent-token table
- redemption receipts

The client owns:

- token randomness
- blind-signature state
- the unblinded signature

Keeping these roles separate makes the privacy boundary easy to inspect in code. The issuer API receives an account and a blinded token. The gateway API receives a token and a signature. There is no endpoint where both account and final token are required.

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

The server uses bbolt for replay state. Each spent token is stored by `SHA256(token)`, not by the raw token bytes. The local tests also keep an in-memory store because it is faster for unit tests.

In a larger deployment, replay protection would be the main consistency problem. The spent-token insert still needs to be a single atomic write:

```sql
INSERT INTO spent_tokens(token_hash, redeemed_at)
VALUES ($1, now())
ON CONFLICT DO NOTHING;
```

The redemption succeeds only if the insert creates a new row. With multiple gateway replicas, this table should be shared or strongly partitioned by token hash.

## Key Rotation

Every blind signature response includes a `key_id`. The gateway keeps a map from `key_id` to issuer public key and its `not_after` time.

The local server loads or creates the issuer RSA key from a PEM file. That avoids losing the signing key on restart. New issuance uses the active key. Gateways can keep old public keys until their `not_after` time passes, and revoked keys can be removed immediately.

This is why the key id is part of the protocol messages even though the demo starts with one key.

## What The System Protects

The gateway cannot learn which account received a token from the normal redemption request. The issuer cannot recognize the final token it signed. A client cannot spend the same token twice once the gateway has recorded the token hash. A client also cannot mint extra valid tokens without getting signatures from the issuer.

The system does not hide timing by itself. If Alice requests a token and immediately uses it, logs from both services may still correlate the events. This is a metadata problem, not a signature problem.

## Remaining Production Work

Quota should move to a ledger or transactional database if this becomes a real product. Signing keys should eventually live in KMS or an HSM instead of a local PEM file. Logs should avoid raw token values and should be checked for timing leaks.

The main design would stay the same: identity at the issuer, anonymous redemption at the gateway, and a one-time replay check in the middle.
