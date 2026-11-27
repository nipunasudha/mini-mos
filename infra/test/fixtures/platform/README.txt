gke-stack plus Argo CD. main.tf, variables.tf and outputs.tf are symlinks to
../gke-stack, argocd.tf is a symlink to envs/dev/argocd.tf, so none of them can
drift from the real thing. Only versions.tf (adds the helm provider) is its own.
