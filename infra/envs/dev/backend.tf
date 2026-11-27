# Partial configuration: the bucket name is not committed. Pass it at init time:
#   terraform init -backend-config="bucket=<state-bucket>"
terraform {
  backend "gcs" {
    prefix = "envs/dev"
  }
}
