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
| Terraform | 1.17.2 (`required_version = "~> 1.17"`) | `infra/**/versions.tf`, CI |
| google provider | 8.12.0 (`~> 8.12`) | `infra/**/versions.tf` + lock files |
| helm provider | 3.3.0 (`~> 3.3`) | `infra/envs/dev`, `infra/test/fixtures/platform` |
| argo-cd chart | 10.20.0 (Argo CD 3.6.3) | `infra/envs/dev/argocd.tf` |
| argocd-apps chart | 2.0.10 | `infra/envs/dev/argocd.tf` |
| Strimzi chart | 1.3.1 | `deploy/platform/strimzi-operator.yaml` |
| Kafka (cluster) | 4.3.2 | `deploy/platform/mos-kafka/kafka.yaml` |
| cloudnative-pg chart | 0.29.2 (operator 1.30.2) | `deploy/platform/cnpg-operator.yaml` |
| kube-prometheus-stack | 94.1.0 | `deploy/platform/kube-prometheus-stack.yaml` |
| terratest / testify | v1.0.1 / v1.12.1 | `infra/test/go.mod` |
| Helm (CLI) | v4.3.2 | CI `azure/setup-helm` |
| yq | v4.54.3 (sha256-pinned in CI) | `.github/workflows/services.yml` |
| Trivy | v0.76.2 | `.github/workflows/services.yml` |
| GitHub Actions | checkout v7.0.1, setup-go v7.1.0, golangci-lint-action v9.3.0, trivy-action v0.36.0, auth v3.0.0, setup-helm v5.1.0, setup-terraform v4.0.1, github-script v9.0.0, upload-artifact v7.0.2 (all SHA-pinned) | `.github/workflows/*.yml` |
