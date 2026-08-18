# Architecture

```text
                         quota upsert
                     +----------------+
                     | PostgreSQL     |
                     | quota_windows  |
                     +----------------+
                              ^
                              |
client -- blinded token --> issuer replicas
client <-- blind signature -- issuer replicas

client -- token + signature --> gateway replicas
client <-- receipt ----------- gateway replicas
                              |
                              v
                     +----------------+
                     | PostgreSQL     |
                     | spent_tokens   |
                     +----------------+
                         atomic insert
```

Issuer replicas share quota state. Gateway replicas share replay state. The account name is part of issuance, not redemption.
