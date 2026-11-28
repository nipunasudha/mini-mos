# Dedicated identity for the nodes. We do not use the Compute default service
# account: new projects often no longer grant it Editor, so nodes would fail to
# write logs or pull images.
resource "google_service_account" "nodes" {
  project      = var.project_id
  account_id   = "${var.name}-nodes"
  display_name = "GKE nodes for ${var.name}"
}

# Logs, metrics and the other things a node needs to join the cluster.
resource "google_project_iam_member" "nodes_default" {
  project = var.project_id
  role    = "roles/container.defaultNodeServiceAccount"
  member  = "serviceAccount:${google_service_account.nodes.email}"
}

# Pull our own images from Artifact Registry. Granted on the project (not the
# repo) because this module does not know the repo; the registry module does.
resource "google_project_iam_member" "nodes_registry_reader" {
  project = var.project_id
  role    = "roles/artifactregistry.reader"
  member  = "serviceAccount:${google_service_account.nodes.email}"
}

resource "google_container_cluster" "this" {
  project  = var.project_id
  name     = var.name
  location = var.location # a zone => zonal Standard cluster

  network    = var.network_id
  subnetwork = var.subnet_id

  # No default node pool: on GKE 1.36.4+ leaving out initial_node_count and
  # remove_default_node_pool creates the cluster without one. (Older versions
  # needed a throwaway pool created and then deleted, which cost minutes.)
  # The only pool is the spot pool below.

  # The demo cluster is destroyed every day.
  deletion_protection = false

  release_channel {
    channel = "REGULAR"
  }

  # Dataplane V2 (Cilium). This is what enforces Kubernetes NetworkPolicy.
  datapath_provider = "ADVANCED_DATAPATH"

  # Workload Identity: pods can act as Google service accounts without keys.
  workload_identity_config {
    workload_pool = "${var.project_id}.svc.id.goog"
  }

  ip_allocation_policy {
    cluster_secondary_range_name  = var.pods_range_name
    services_secondary_range_name = var.services_range_name
  }

  # Nodes get only private IPs. The control-plane endpoint stays public so the
  # operator (and Terraform's helm provider) can reach it, but only from the
  # authorised CIDRs below. CI never talks to the cluster.
  private_cluster_config {
    enable_private_nodes    = true
    enable_private_endpoint = false
    master_ipv4_cidr_block  = var.master_ipv4_cidr_block
  }

  master_authorized_networks_config {
    dynamic "cidr_blocks" {
      for_each = var.master_authorized_cidrs
      content {
        cidr_block   = cidr_blocks.value
        display_name = "authorised-${cidr_blocks.key}"
      }
    }
  }
}

resource "google_container_node_pool" "spot" {
  # Paused: no nodes, so nothing is billed except what the GKE free tier covers
  # (one zonal control plane) and the Postgres disk. Unpausing recreates the pool;
  # everything in the cluster (Argo CD, the apps, the Postgres volume) is still there.
  count = var.paused ? 0 : 1

  project  = var.project_id
  name     = "spot"
  location = var.location
  cluster  = google_container_cluster.this.name

  # Not node_count: it conflicts with autoscaling, which then owns the size.
  initial_node_count = var.node_count

  autoscaling {
    min_node_count = 2
    max_node_count = 4
  }

  node_config {
    machine_type = var.machine_type
    spot         = true # cheap, and can be reclaimed at any time (fine for a demo)

    # GKE's default boot disk is 100 GB pd-balanced, which counts against the regional
    # SSD quota (250 GB on a fresh project: 3 nodes don't fit). A node only holds the OS
    # and container images (~10-15 GB), so 20 GB pd-standard is enough and uses the much
    # larger HDD quota. Raise it if nodes report DiskPressure.
    disk_size_gb    = 20
    disk_type       = "pd-standard"
    service_account = google_service_account.nodes.email

    # Access is controlled by IAM roles on the service account, not by scopes.
    oauth_scopes = ["https://www.googleapis.com/auth/cloud-platform"]

    shielded_instance_config {
      enable_secure_boot          = true
      enable_integrity_monitoring = true
    }

    # Serve pods the Workload Identity metadata server, not the node's own SA.
    workload_metadata_config {
      mode = "GKE_METADATA"
    }
  }

  # The node SA needs its roles before nodes boot, or they cannot pull images.
  depends_on = [
    google_project_iam_member.nodes_default,
    google_project_iam_member.nodes_registry_reader,
  ]
}
