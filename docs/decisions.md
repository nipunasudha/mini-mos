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
| 2026-11-26 | E2E tests in Go (`tests/e2e`), not a shell script | Real assertions, polling with timeouts, readable failures; same code runs against compose and GKE | `acceptance.sh` in bash (brittle; kept only as `scripts/demo.sh` for show) |
| 2026-11-26 | Test-only admin endpoint to create clients (`ADMIN_API_ENABLED`) | Isolated, repeatable e2e runs and demo rehearsals on a long-lived DB | Fixed seed + run-order-dependent tests; direct DB access from tests |
| 2026-11-26 | Terratest in three tiers behind build tags (static, plan, provisioning) | Static/plan tiers are free and safe for CI; the apply tier costs money and needs admin rights | One all-or-nothing infra test; no infra tests |
| 2026-11-26 | Terratest apply tier runs locally only | Avoids granting CI apply rights to the GCP project | `workflow_dispatch` job with an admin SA |
| 2026-11-26 | External accounts are created by a migration; their `balance_cents` is never updated (entries still recorded) | Updating them would make one hot row that every client's transaction locks; the balance is `SUM(ledger_entries)` | Running balance on external accounts |
| 2026-11-26 | Seed bill IDs are UUIDv5 of `client\|pot\|due-offset`; one `SeedClient` serves `seed` and the admin endpoint | Deterministic IDs make `ON CONFLICT DO NOTHING` truly idempotent | Random IDs (re-seed duplicates bills) |
| 2026-11-26 | Idempotency row inserted first with placeholders, updated at the end; the response is built from the stored JSONB | Replays are byte-identical to the original even though JSONB reorders keys | Answer from the pre-store marshal |
| 2026-11-26 | Request hash = sha256 of the re-marshalled parsed body (bill ID for payments) | Whitespace and key order don't change the hash | Hash the raw body |
| 2026-11-26 | A failed produce rolls back the whole relay batch | Keeps per-client order; resent records are absorbed by at-least-once consumers | Commit the rows that succeeded |
| 2026-11-26 | Migrations take a Postgres advisory lock (`goose.WithSessionLocker`) | Init containers of two replicas can run `migrate` at once | Unlocked migrations |
| 2026-11-26 | `due_before` is inclusive (`due_date <= value`) | Simpler for callers; seed data is unaffected | Exclusive bound |
| 2026-11-26 | Scheduler commits each record's offset only after handling it; the in-flight record finishes on SIGTERM, then the consumer leaves the group | At-least-once with no half-handled records; a stale member can't block an offset reset | Auto-commit; batch commits |
| 2026-11-26 | Scheduler stops at the first bill that fails after retries and DLQs the whole record | An outage would fail the remaining bills too; paid bills replay as no-ops | Skip the bill and continue |
| 2026-11-26 | Backoff `base·2^(n-1)`, jittered into the upper half | Retries spread across replicas while every wait stays at least half the base | Full jitter (can wait ~0) |
| 2026-11-26 | Kafka `auto.create.topics.enable=false`; a `topic-init` job creates topics | A wrong topic name fails loudly instead of creating a 1-partition topic | Auto-create |
| 2026-11-26 | e2e suite is one `TestE2E`: a parallel group (tests 1–4), then redelivery | Go runs parallel top-level tests after serial ones, so a serial redelivery test would run first | Top-level serial test; `-parallel 1` |
| 2026-11-26 | `redeliver.sh` retries the offset reset, then waits for lag 0 | Kafka refuses to reset a group with members, and a killed consumer lingers until session timeout | Single reset attempt |
| 2026-11-27 | Root app is **not** recursive over `deploy/platform` | Subfolders are their own child Applications; recursing would make two apps own the same resources | Recursive root (as first specified) |
| 2026-11-27 | Application health re-enabled via `resource.customizations.health.argoproj.io_Application` | Sync waves across child apps only wait if Argo CD can judge their health | Relying on waves alone |
| 2026-11-27 | `mos` Namespace is a plain manifest at wave -4 | The Strimzi chart creates RoleBindings in `mos` at wave -3, so it must exist first | `CreateNamespace=true` on a child app (too late) |
| 2026-11-27 | ApplicationSet ignores differences in `/spec/syncPolicy` | Self-heal reverts `kubectl scale --replicas=0` in seconds; redeliver and the lag demo pause auto-sync on bill-scheduler only, and Kafka refuses an offset reset while the group has members | Turning self-heal off everywhere; ignoring `/spec/replicas` |
| 2026-11-27 | NetworkPolicy egress is simplified: DNS, Kafka, Postgres, and `part-of: mini-mos` pods on 8080 | One chart render can't see which other apps list this one in `allowFrom`; ingress (the callee side) stays strict | Per-app egress lists computed across services |
| 2026-11-27 | Pod `runAsUser/runAsGroup: 65532` | The images use `USER nonroot` by name, and the kubelet can't verify `runAsNonRoot` against a name | Numeric USER in each Dockerfile |
| 2026-11-27 | Kafka storage `ephemeral`; Prometheus without a PVC (2d retention) | Deleting a GKE cluster leaves dynamically provisioned disks billing; a broker restart only causes harmless redelivery | Persistent volumes (Postgres keeps its 5Gi PVC per spec) |
| 2026-11-27 | `ServerSideApply=true` on all three operator apps | Their CRDs exceed the client-side apply annotation limit | Client-side apply |
| 2026-11-27 | Control-plane scrapes GKE doesn't expose (etcd, scheduler, controller-manager, kube-proxy, coreDns) are disabled | They'd be permanently-down targets and false alerts | Chart defaults |
| 2026-11-27 | Strimzi 1.3.1 runs Kafka 4.3.2 while compose runs 4.4.0 | 4.3.2 is the newest that Strimzi release supports; the client protocol is the same | Waiting for Strimzi to support 4.4 |
| 2026-11-27 | Subnet has Private Google Access; Cloud NAT kept | Artifact Registry pulls stay on Google's network; NAT is for Docker Hub, Quay and GitHub | NAT for everything |
| 2026-11-27 | Dedicated node service account (`container.defaultNodeServiceAccount` + `artifactregistry.reader`) | New projects often don't grant the Compute default SA Editor | Compute default SA |
| 2026-11-27 | APIs and the state bucket are created once by hand with gcloud; backend bucket passed via `-backend-config` | Terraform can't create its own backend; disabling APIs on destroy would break the next apply; nothing project-specific in the backend file | `google_project_service` in Terraform; hard-coded bucket |
| 2026-11-27 | Registry and fixtures take a name suffix | Deleted WIF pools and service accounts reserve their IDs for 30 days; tests must not collide with real names | Fixed names |
| 2026-11-27 | No `kubernetes` Terraform provider | Nothing needs it: the argo-cd chart creates its namespace and Argo CD creates the rest | Declaring an unused provider |
| 2026-11-27 | Terratest NAT probe pulls from Quay | GKE can pull Docker Hub images through Google's mirror, which could hide a missing NAT | Docker Hub probe |
| 2026-11-27 | `services.yml` adds `workflow_dispatch` (builds every service) | The first build after `terraform apply` needs to build everything without a code change | Dummy commits |
| 2026-11-27 | Promote runs only if every build leg succeeded; one commit per service; push retried once after `git pull --rebase` | A half-failed matrix must not deploy; concurrent merges race on main | Promote whatever built |
| 2026-11-27 | `infra-plan.yml` plan job is advisory and runs only when `PLAN_SA` is set | The read-only plan SA may not read every IAM policy; static tier is the required gate | Blocking plan job |
| 2026-11-27 | GCP project ID appears in the deploy values (image repository) | GitOps needs the full image reference in Git; project IDs aren't credentials | Injecting the registry at sync time |
