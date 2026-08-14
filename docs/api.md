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
  "e": 65537
}
```

Clients use this key to blind tokens locally.

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
  "signature": "hex-blind-signature"
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
- `409 already_spent`
