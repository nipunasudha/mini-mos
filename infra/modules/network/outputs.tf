output "network_id" {
  value = google_compute_network.this.id
}

output "subnet_id" {
  value = google_compute_subnetwork.this.id
}

output "pods_range_name" {
  value = "pods" # must match the secondary_ip_range names in main.tf
}

output "services_range_name" {
  value = "services"
}
