output "cluster_name" {
  value = module.gke.cluster_name
}

output "get_credentials_command" {
  description = "Run this to point kubectl at the cluster."
  value       = "gcloud container clusters get-credentials ${module.gke.cluster_name} --zone ${var.zone} --project ${var.project_id}"
}

output "registry_url" {
  description = "Image prefix for the services."
  value       = module.registry.repository_url
}

output "wif_provider" {
  description = "GitHub repo variable WIF_PROVIDER."
  value       = module.registry.wif_provider_name
}

output "pusher_sa_email" {
  description = "GitHub repo variable PUSHER_SA."
  value       = module.registry.pusher_sa_email
}

output "plan_sa_email" {
  description = "GitHub repo variable PLAN_SA."
  value       = module.registry.plan_sa_email
}
