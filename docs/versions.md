# Pinned versions

Current stable at build time. Update this file whenever a pin changes.

| Component | Version | Pinned in |
| --- | --- | --- |
| Go | 1.27.4 | `services/*/go.mod`, `tests/e2e/go.mod`, Dockerfiles (`golang:1.27.4`) |
| Runtime image | `gcr.io/distroless/static-debian12:nonroot` | Dockerfiles |
| go-chi/chi | v5.5.0 | `services/ledger-api/go.mod` |
| jackc/pgx | v5.13.0 | `services/ledger-api/go.mod` |
| pressly/goose | v3.30.2 | `services/ledger-api/go.mod` |
| twmb/franz-go | v1.22.3 | `services/*/go.mod` |
| prometheus/client_golang | v1.25.0 | `services/*/go.mod` |
| google/uuid | v1.6.0 | `services/*/go.mod` |
| testcontainers-go (+ postgres module) | v0.45.0 | `services/ledger-api/go.mod` |
| Postgres (compose) | 19.0 | `docker-compose.yml` |
| Postgres (integration tests) | 19.0-alpine | `services/ledger-api/internal/store/store_integration_test.go` |
| Kafka | apache/kafka 4.4.0 | `docker-compose.yml` |
| golangci-lint | v2.15.0 | `Makefile` |
