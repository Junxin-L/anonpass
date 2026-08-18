# Contributing

Keep changes small and easy to review.

Before opening a pull request:

```sh
gofmt -w $(find . -name '*.go')
go vet ./...
go test ./...
go test -race ./...
```

For security-sensitive changes, include tests for the failure mode. Examples:

- replay attempts must return `already_spent`
- quota exhaustion must return `no_quota`
- expired keys must return `expired_key`
- malformed hex must return a 400 response

Do not log raw tokens, raw signatures, private keys, or account-linked redemption data.

## Style

Use plain Go and keep package boundaries clear:

- `internal/blindrsa`: thin wrapper around CIRCL
- `internal/tokens`: issuer, gateway, quota, replay stores
- `internal/httpapi`: JSON API and demo endpoints
- `internal/webui`: embedded browser console

Prefer direct names over clever abstractions.
