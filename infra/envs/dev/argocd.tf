# Terraform only installs Argo CD and one root Application. Everything else in
# the cluster is installed by Argo CD from Git.
# test/fixtures/platform symlinks this file, so the Terratest full-stack tier
# installs exactly what dev installs.

data "google_client_config" "default" {}

# Credentials come from the cluster we just created plus a short-lived token for
# whoever runs Terraform, so no kubeconfig is needed.
provider "helm" {
  kubernetes = {
    host                   = "https://${module.gke.endpoint}"
    token                  = data.google_client_config.default.access_token
    cluster_ca_certificate = base64decode(module.gke.ca_certificate)
  }
}

resource "helm_release" "argocd" {
  name             = "argocd"
  namespace        = "argocd"
  create_namespace = true
  repository       = "https://argoproj.github.io/argo-helm"
  chart            = "argo-cd"
  version          = "10.20.0"
  timeout          = 600

  values = [yamlencode({
    configs = {
      cm = {
        # Argo CD 1.8+ stopped reporting health for Application resources. Sync
        # waves between child Applications need it: without it a wave -3 child
        # counts as done instantly and wave -2 starts before the operators exist.
        # This is the snippet from the Argo CD docs: copy the child's own health.
        "resource.customizations.health.argoproj.io_Application" = <<-LUA
          hs = {}
          hs.status = "Progressing"
          hs.message = ""
          if obj.status ~= nil then
            if obj.status.health ~= nil then
              hs.status = obj.status.health.status
              if obj.status.health.message ~= nil then
                hs.message = obj.status.health.message
              end
            end
          end
          return hs
        LUA
      }
    }
  })]

  # The node pool must exist before Helm waits for pods to schedule.
  depends_on = [module.gke]
}

# The root Application ("app of apps"). It points at deploy/platform, whose YAML
# files are the child Applications.
resource "helm_release" "root_app" {
  name       = "argocd-apps"
  namespace  = "argocd"
  repository = "https://argoproj.github.io/argo-helm"
  chart      = "argocd-apps"
  version    = "2.0.10"

  values = [yamlencode({
    applications = {
      root = {
        namespace = "argocd"
        project   = "default"
        source = {
          repoURL        = "https://github.com/${var.github_repo}.git"
          targetRevision = "main"
          path           = "deploy/platform"
          # Not recursive on purpose: deploy/platform/mos-kafka and friends are
          # their own child Applications. Recursing would make the root and the
          # child both own the same resources.
          directory = { recurse = false }
        }
        destination = {
          server    = "https://kubernetes.default.svc"
          namespace = "argocd"
        }
        syncPolicy = {
          automated = {
            prune    = true
            selfHeal = true
          }
        }
      }
    }
  })]

  # The Application CRD comes from the argo-cd chart.
  depends_on = [helm_release.argocd]
}
