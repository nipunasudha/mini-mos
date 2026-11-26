# mini-MOS

A small money-management platform: Go services that move client money safely (idempotency keys, transactional outbox, at-least-once Kafka, DB uniqueness backstop), deployed to private GKE with Terraform and Argo CD, observed with Prometheus and Grafana.

> Work in progress. Architecture diagram, quickstart, design choices and "What I'd do next" land as the milestones are built.

## Layout

| Path | Contents |
| --- | --- |
| `services/` | Go services, one `go.mod` each |
| `charts/service/` | the shared Helm chart every service deploys with |
| `deploy/` | Argo CD apps: platform (operators, Kafka, Postgres, observability) and per-service values |
| `infra/` | Terraform: network, GKE, Artifact Registry + Workload Identity Federation |
| `docs/` | [decisions](docs/decisions.md), [versions](docs/versions.md), runbook |
