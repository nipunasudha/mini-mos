output "repository_id" {
  value = google_artifact_registry_repository.this.repository_id
}

output "repository_url" {
  description = "Image prefix: <region>-docker.pkg.dev/<project>/<repo>"
  value       = "${var.region}-docker.pkg.dev/${var.project_id}/${google_artifact_registry_repository.this.repository_id}"
}

output "wif_provider_name" {
  description = "Full provider resource name; this is the WIF_PROVIDER GitHub variable."
  value       = google_iam_workload_identity_pool_provider.github.name
}

output "pusher_sa_email" {
  value = google_service_account.pusher.email
}

output "plan_sa_email" {
  value = google_service_account.plan.email
}
