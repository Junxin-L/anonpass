# API

## Get Issuer Key

```http
GET /v1/issuer/key
```

Response:

```json
{
  "key_id": "local-1",
  "n": "hex-rsa-modulus",
  "e": 65537,
  "not_after": 1786636800
}
```

Clients use this key to blind tokens locally. Gateways reject redemption after `not_after`.

## Blind Sign

```http
POST /v1/issuer/blind-sign
```

Request:

```json
{
  "account": "alice",
  "blinded_token": "hex"
}
```

Response:

```json
{
  "key_id": "local-1",
  "signature": "hex-blind-signature",
  "remaining": 4
}
```

Errors:

- `400 bad_json`
- `400 missing_field`
- `400 bad_blinded_token`
- `429 no_quota`

## Redeem

```http
POST /v1/gateway/redeem
```

Request:

```json
{
  "key_id": "local-1",
  "token": "hex-token",
  "signature": "hex-unblinded-signature"
}
```

Response:

```json
{
  "token_hash": "sha256-token-hex",
  "redeemed_at": 1786550400
}
```

Errors:

- `400 unknown_key`
- `401 bad_token`
- `401 expired_key`
- `409 already_spent`

## Demo Issue

```http
POST /v1/demo/issue
```

Request:

```json
{
  "account": "alice@example.com"
}
```

Response:

```json
{
  "id": "demo-session-id",
  "account": "alice@example.com",
  "key_id": "local-1",
  "remaining": 4,
  "token": "hex-prepared-token",
  "blinded_token": "hex-blinded-token",
  "blind_signature": "hex-blind-signature",
  "signature": "hex-unblinded-signature"
}
```

This endpoint is for the browser console. It simulates the client-side blind and unblind steps on the server so the protocol can be inspected without a JavaScript cryptography implementation.

## Demo Redeem

```http
POST /v1/demo/redeem
```

Request:

```json
{
  "session_id": "demo-session-id"
}
```

The first call should return a receipt. A second call with the same session should return `409 already_spent`.
