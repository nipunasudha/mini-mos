# Runbook

Bootstrap, apply/destroy and Terratest. (Demo-day checklist and troubleshooting are added in Milestone 4.)

Needs: `gcloud` (logged in), `gke-gcloud-auth-plugin` (`gcloud components install gke-gcloud-auth-plugin`), `terraform` 1.17+, `kubectl`, `gh`, `jq`, Go 1.27.

## 1. One-time bootstrap

Done once per GCP project, by hand. Terraform does not manage the APIs or the state bucket: it cannot create its own backend, and disabling an API on destroy would break the next apply.

```sh
export PROJECT=<your-project-id>
export REGION=asia-south1
export BUCKET=$PROJECT-mini-mos-tfstate

gcloud auth login
gcloud auth application-default login      # Terraform and Terratest use these credentials
gcloud config set project $PROJECT

gcloud services enable \
  container.googleapis.com compute.googleapis.com artifactregistry.googleapis.com \
  iam.googleapis.com iamcredentials.googleapis.com sts.googleapis.com \
  cloudresourcemanager.googleapis.com

# State bucket, with versioning so a bad apply can be rolled back.
gcloud storage buckets create gs://$BUCKET --location=$REGION --uniform-bucket-level-access
gcloud storage buckets update gs://$BUCKET --versioning
```

Then configure `envs/dev`:

```sh
cd infra/envs/dev
cp terraform.tfvars.example terraform.tfvars   # git-ignored; set project_id, state_bucket, your IP
terraform init -backend-config="bucket=$BUCKET"
```

`master_authorized_cidrs` must contain your current public IP (`curl -s https://checkip.amazonaws.com`), because Terraform installs Argo CD through the cluster's public endpoint. If your IP changes, update the tfvars and apply again.

## 2. Apply

```sh
cd infra/envs/dev
terraform apply
```

Empty project to Argo CD running: **to be measured** (spec estimate: ~15 minutes). Argo CD then installs everything else from `deploy/platform` on `main`; until that folder exists on `main`, the root app shows an error. Time until every app is Synced/Healthy: **to be measured**.

```sh
eval "$(terraform output -raw get_credentials_command)"   # kubectl now points at the cluster
```

## 3. GitHub repo variables

CI authenticates to GCP with Workload Identity Federation: no keys are stored. Set these once, from the outputs:

```sh
R=nipunasudha/mini-mos
gh variable set WIF_PROVIDER --repo $R --body "$(terraform output -raw wif_provider)"
gh variable set PUSHER_SA    --repo $R --body "$(terraform output -raw pusher_sa_email)"
gh variable set PLAN_SA      --repo $R --body "$(terraform output -raw plan_sa_email)"
gh variable set GCP_PROJECT  --repo $R --body "$PROJECT"
gh variable set GCP_REGION   --repo $R --body "$REGION"
```

These values change only if the registry module is destroyed and recreated. `PLAN_SA` can read state but not write it, so a CI `terraform plan` must use `-lock=false`.

## 4. Argo CD UI

```sh
kubectl -n argocd port-forward svc/argocd-server 8080:443
# user: admin   password:
kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath='{.data.password}' | base64 -d; echo
```

Open https://localhost:8080 (accept the self-signed certificate).

## 5. Destroy

**Pre-destroy step (do not skip).** Deleting a GKE cluster does not delete the persistent disks that Kafka and Postgres claimed. They keep costing money, and nothing in Terraform knows about them. Remove the workloads first so Kubernetes deletes the disks:

```sh
# Stop anything that would recreate resources: root app, then the ApplicationSet, then child apps.
kubectl -n argocd delete application root --wait --timeout=5m
kubectl -n argocd delete applicationset --all --wait --timeout=5m
kubectl -n argocd delete application --all --wait --timeout=10m
# Pods and PVCs (and so their disks) live in mos.
kubectl delete namespace mos --wait --timeout=10m
kubectl get pv                      # expect: No resources found

gcloud compute disks list --filter="name~^pvc-"   # expect: empty (or only disks from other clusters)
```

If a disk is left, delete it by hand: `gcloud compute disks delete <name> --zone asia-south1-a`. Only then:

```sh
cd infra/envs/dev
terraform destroy
```

Destroy time: **to be measured**. After it, only the state bucket and the Artifact Registry images should remain: check `gcloud container clusters list`, `gcloud compute disks list`, `gcloud compute networks list`.

## 6. Terratest

All tests live in `infra/test/`. Each tier is a separate command; build tags stop the costly tiers from running by accident.

| Tier | Command | Needs | Cost / time |
| --- | --- | --- | --- |
| Static | `make infra-test-static` | nothing | free, ~20s (to be measured in CI) |
| Plan | `GOOGLE_PROJECT=$PROJECT make infra-test-plan` | ADC | free, ~1 min (to be measured) |
| Live | `GOOGLE_PROJECT=$PROJECT make infra-test-live` | ADC with apply rights | one cluster, 20-25 min (to be measured) |
| Full stack | `cd infra/test && go test -v -count=1 -timeout 90m -tags provisioning -run TestPlatformBootstrap ./...` | same | one cluster, 35-45 min (to be measured) |

- Static: `terraform init -backend=false` + `validate` on every module, fixture and `envs/dev`, plus `terraform fmt -check -recursive`. CI runs it on every infra PR.
- Plan: asserts on the planned resources (spot pool 2-4, default pool removed, private nodes, Workload Identity, Dataplane V2, WIF condition, registry format). Creates nothing.
- Live: applies `fixtures/gke-stack` with a random name suffix, then checks the real cluster (spot nodes Ready, no external IPs, image pull through NAT, NetworkPolicy enforced, registry exists) and always destroys. Fixtures use their own local state, so `envs/dev` is never touched. The test needs your public IP in the authorised networks: it detects it, or set `MASTER_AUTHORIZED_CIDR=<ip>/32`.
- Full stack: also installs Argo CD and the root app from `main`, waits for every Application to be Synced/Healthy, runs the pre-destroy step from section 5, destroys and checks that no PV disks are left. It doubles as the demo-day rehearsal and gives the timings above.

### Iterating without paying for a new cluster each time

Stages are `setup`, `validate`, `teardown`. A `SKIP_<stage>=true` environment variable skips it:

```sh
cd infra/test
export GOOGLE_PROJECT=$PROJECT
SKIP_teardown=true go test -v -timeout 60m -tags provisioning -run TestGKEStackProvisioning ./...   # build, validate, keep
SKIP_setup=true SKIP_teardown=true go test -v -timeout 60m -tags provisioning -run TestGKEStackProvisioning ./...   # re-run assertions only
SKIP_setup=true SKIP_validate=true go test -v -timeout 60m -tags provisioning -run TestGKEStackProvisioning ./...   # destroy the kept cluster
```

The saved options (including the random suffix) live in `infra/test/fixtures/<fixture>/.test-data/` (git-ignored). Do not forget the last command: a kept cluster is billed by the hour.

### Cleanup after a killed run

Go's default `-timeout` is 10 minutes and kills the test in the middle of an apply; a Ctrl-C does the same. The `defer`red destroy does not run. The fixture's state is local, in its own directory, so destroy from there with the variables Terratest saved:

```sh
cd infra/test/fixtures/gke-stack          # or fixtures/platform
V=.test-data/TerraformOptions.json
terraform init
terraform destroy \
  -var project_id=$PROJECT \
  -var suffix="$(jq -r .Vars.suffix $V)" \
  -var "master_authorized_cidrs=$(jq -c .Vars.master_authorized_cidrs $V)"
```

For `fixtures/platform`, run the pre-destroy step from section 5 first (against that cluster). Then confirm nothing is left in the console or with:

```sh
gcloud container clusters list
gcloud compute disks list
gcloud compute networks list
gcloud artifacts repositories list
gcloud iam service-accounts list
```

Deleted Workload Identity pools and service accounts keep their IDs reserved for 30 days. This is why fixtures add a random suffix to every name.
