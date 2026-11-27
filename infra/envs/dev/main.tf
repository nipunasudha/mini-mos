provider "google" {
  project = var.project_id
  region  = var.region
}

module "network" {
  source = "../../modules/network"

  project_id = var.project_id
  region     = var.region
  name       = "mini-mos"
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
}

module "registry" {
  source = "../../modules/registry"

  project_id   = var.project_id
  region       = var.region
  github_repo  = var.github_repo
  state_bucket = var.state_bucket
}
