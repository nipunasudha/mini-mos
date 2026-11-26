# Decisions

ADR-lite log: one row per non-obvious decision.

| Date | Decision | Why | Rejected |
| --- | --- | --- | --- |
| 2026-11-26 | Monorepo | One place to review; Argo CD watches a folder | Separate app/gitops/infra repos (more realistic, more overhead for 3 days) |
| 2026-11-26 | Relay goroutine inside ledger-api | Fewer moving parts; still a real transactional outbox | Separate outbox-relay service; Debezium CDC |
| 2026-11-26 | Idempotency key **and** `UNIQUE(bill_id)` | Defence in depth: the API contract plus a DB invariant | Key only |
| 2026-11-26 | Don't store `insufficient_funds` responses | A retry must succeed once money arrives | Store every response |
| 2026-11-26 | Strimzi and CNPG in-cluster | Shows operators/CRDs; no managed-service cost | Managed Kafka, Cloud SQL (named as production choice) |
| 2026-11-26 | Single broker / single PG instance | Cost; demo only | HA (talking point) |
| 2026-11-26 | Kafka replication factor 1, `min.insync.replicas` 1 | Single broker can't do more; demo only | RF 3 / min ISR 2 (production) |
| 2026-11-26 | Zonal Standard GKE on spot nodes | Cheap, Kafka-friendly; Autopilot pod-resource rules add friction | Autopilot |
| 2026-11-26 | Public control plane limited to owner IP | CI uses pull-based GitOps and never needs the API | Fully private endpoint + bastion |
| 2026-11-26 | No CPU limits | Avoid CFS throttling; memory limits kept | CPU limits everywhere |
| 2026-11-26 | JSON events, no schema registry | Scope | Avro/Protobuf + registry (next step) |
| 2026-11-26 | No shared Go library between services | Duplicating ~30 lines of config/metrics helpers keeps services independent (self-service story) | `pkg/` shared module |
