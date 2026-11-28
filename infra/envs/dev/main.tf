provider "google" {
  project = var.project_id
  region  = var.region
}

# APIs this stack needs. disable_on_destroy = false: destroying the stack must
# never switch an API off (that would break other users of the project and the
# next apply). Terraform only ever turns them on.
resource "google_project_service" "enabled" {
  for_each = toset([
    "compute.googleapis.com",
    "container.googleapis.com",
    "artifactregistry.googleapis.com",
    "iam.googleapis.com",
    "iamcredentials.googleapis.com", # WIF token exchange at CI run time
    "sts.googleapis.com",            # same
    "cloudresourcemanager.googleapis.com",
  ])

  project            = var.project_id
  service            = each.value
  disable_on_destroy = false
}

module "network" {
  source = "../../modules/network"

  project_id = var.project_id
  region     = var.region
  name       = "mini-mos"

  depends_on = [google_project_service.enabled]
}

module "gke" {
  source = "../../modules/gke"

  project_id              = var.project_id
  name                    = "mini-mos"
  location                = var.zone
  network_id              = module.network.network_id
  subnet_id               = module.network.subnet_id
  pods_range_name         = module.network.pods_range_name
  services_range_name     = module.network.services_range_name
  master_authorized_cidrs = var.master_authorized_cidrs
  paused                  = var.paused

  depends_on = [google_project_service.enabled]
}

module "registry" {
  source = "../../modules/registry"

  project_id   = var.project_id
  region       = var.region
  github_repo  = var.github_repo
  state_bucket = var.state_bucket

  depends_on = [google_project_service.enabled]
}
