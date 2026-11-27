# Custom-mode VPC: we choose every range ourselves instead of getting one
# auto-created subnet per region.
resource "google_compute_network" "this" {
  project                 = var.project_id
  name                    = "${var.name}-vpc"
  auto_create_subnetworks = false
}

resource "google_compute_subnetwork" "this" {
  project = var.project_id
  name    = "${var.name}-subnet"
  region  = var.region
  network = google_compute_network.this.id

  ip_cidr_range = "10.10.0.0/20" # node IPs

  # Private nodes have no public IP. With this on, they still reach Google APIs
  # (including Artifact Registry) directly, so image pulls from our own registry
  # do not depend on NAT.
  private_ip_google_access = true

  # GKE VPC-native clusters take pod and service IPs from these named ranges.
  secondary_ip_range {
    range_name    = "pods"
    ip_cidr_range = "10.20.0.0/16"
  }

  secondary_ip_range {
    range_name    = "services"
    ip_cidr_range = "10.30.0.0/20"
  }
}

# Cloud NAT is for everything that is not Google: Docker Hub, Quay, GHCR and
# GitHub (Argo CD pulls the repo from there). Without it, private nodes get
# ImagePullBackOff.
resource "google_compute_router" "this" {
  project = var.project_id
  name    = "${var.name}-router"
  region  = var.region
  network = google_compute_network.this.id
}

resource "google_compute_router_nat" "this" {
  project = var.project_id
  name    = "${var.name}-nat"
  region  = var.region
  router  = google_compute_router.this.name

  nat_ip_allocate_option = "AUTO_ONLY"

  # ALL ranges: pods egress with their own pod IPs, so the secondary ranges
  # must be NATed too, not just the node range.
  source_subnetwork_ip_ranges_to_nat = "ALL_SUBNETWORKS_ALL_IP_RANGES"
}
