output "cluster_name" {
  value = module.gke.cluster_name
}

output "zone" {
  value = var.zone
}

output "repository_id" {
  value = module.registry.repository_id
}
