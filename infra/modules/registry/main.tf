resource "google_artifact_registry_repository" "this" {
  project       = var.project_id
  location      = var.region
  repository_id = "mini-mos${var.name_suffix}"
  format        = "DOCKER"
  description   = "mini-MOS service images"
}

# Workload Identity Federation: GitHub Actions proves who it is with a short-lived
# OIDC token, and Google trades it for a short-lived credential. No SA key to leak.
resource "google_iam_workload_identity_pool" "github" {
  project                   = var.project_id
  workload_identity_pool_id = "github${var.name_suffix}"
  display_name              = "GitHub Actions"
}

resource "google_iam_workload_identity_pool_provider" "github" {
  project                            = var.project_id
  workload_identity_pool_id          = google_iam_workload_identity_pool.github.workload_identity_pool_id
  workload_identity_pool_provider_id = "github-oidc"

  oidc {
    issuer_uri = "https://token.actions.githubusercontent.com"
  }

  attribute_mapping = {
    "google.subject"       = "assertion.sub"
    "attribute.repository" = "assertion.repository"
  }

  # Any GitHub repo can get a token from that issuer. This line is what limits
  # the provider to ours; without it anyone on GitHub could use the pool.
  attribute_condition = "assertion.repository == \"${var.github_repo}\""
}

# Who may impersonate the service accounts below: tokens from our repo only.
locals {
  repo_principal = "principalSet://iam.googleapis.com/${google_iam_workload_identity_pool.github.name}/attribute.repository/${var.github_repo}"
}

# --- gha-pusher: CI pushes images ---

resource "google_service_account" "pusher" {
  project      = var.project_id
  account_id   = "gha-pusher${var.name_suffix}"
  display_name = "GitHub Actions image pusher"
}

# Writer on this one repo, not on the project.
resource "google_artifact_registry_repository_iam_member" "pusher_writer" {
  project    = var.project_id
  location   = google_artifact_registry_repository.this.location
  repository = google_artifact_registry_repository.this.name
  role       = "roles/artifactregistry.writer"
  member     = "serviceAccount:${google_service_account.pusher.email}"
}

resource "google_service_account_iam_member" "pusher_wif" {
  service_account_id = google_service_account.pusher.name
  role               = "roles/iam.workloadIdentityUser"
  member             = local.repo_principal
}

# --- gha-tf-plan: CI runs terraform plan, read-only ---

resource "google_service_account" "plan" {
  project      = var.project_id
  account_id   = "gha-tf-plan${var.name_suffix}"
  display_name = "GitHub Actions terraform plan"
}

# Plan has to read every resource to diff it against state.
resource "google_project_iam_member" "plan_viewer" {
  project = var.project_id
  role    = "roles/viewer"
  member  = "serviceAccount:${google_service_account.plan.email}"
}

# ...and to read the state. Skipped when no bucket is given (test fixtures).
resource "google_storage_bucket_iam_member" "plan_state_reader" {
  count = var.state_bucket == "" ? 0 : 1

  bucket = var.state_bucket
  role   = "roles/storage.objectViewer"
  member = "serviceAccount:${google_service_account.plan.email}"
}

resource "google_service_account_iam_member" "plan_wif" {
  service_account_id = google_service_account.plan.name
  role               = "roles/iam.workloadIdentityUser"
  member             = local.repo_principal
}
