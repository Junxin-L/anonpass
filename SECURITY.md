# Security

Anonpass is a security project, but it is still a small reference implementation. Please report bugs privately before opening a public issue.

## Supported Branch

Security fixes are made on `main`.

## Reporting

Send a short report with:

- affected commit
- reproduction steps
- expected result
- observed result
- whether the issue affects privacy, replay protection, quota enforcement, or key handling

Do not include private keys, live credentials, or production token values in the report.

## Security Scope

In scope:

- replay acceptance
- quota over-issuance
- signature verification bypass
- key expiry bypass
- account identity leakage at the gateway boundary
- unsafe logging of token material
- denial of service caused by malformed requests

Out of scope for this repository:

- browser UI cosmetic issues
- attacks requiring local write access to the issuer private key
- timing correlation across external infrastructure not controlled by this service

## Current Security Posture

The signature scheme uses Cloudflare CIRCL's RFC 9474 RSA Blind Signature implementation. The service adds quota checks, key ids, key expiry, and replay storage.

For multi-replica deployments, use PostgreSQL-backed quota and replay stores. The local bbolt replay store is intended for single-node deployments.
