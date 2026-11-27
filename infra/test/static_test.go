package test

import (
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terratest/modules/terraform"
	"github.com/stretchr/testify/require"
)

// Static tier: free, needs no credentials, safe for every PR.
// It proves every Terraform root and module is syntactically valid and
// consistently formatted.

// Every directory that holds Terraform code we own. A new module or fixture
// must be added here, otherwise CI would not validate it.
var terraformDirs = []string{
	"../modules/network",
	"../modules/gke",
	"../modules/registry",
	"../envs/dev",
	"fixtures/gke-stack",
	"fixtures/platform",
}

func TestModulesValidate(t *testing.T) {
	// One provider download shared by all directories instead of one each.
	cache := t.TempDir()

	// Formatting is checked once, recursively, from infra/.
	t.Run("fmt", func(t *testing.T) {
		opts := &terraform.Options{TerraformDir: "..", NoColor: true}
		_, err := terraform.RunTerraformCommandContextE(t, t.Context(), opts, "fmt", "-check", "-recursive", "-diff")
		require.NoError(t, err, "run `terraform fmt -recursive infra` and commit the result")
	})

	for _, dir := range terraformDirs {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			opts := &terraform.Options{
				TerraformDir: dir,
				NoColor:      true,
				// envs/dev declares a gcs backend; validating must not need the bucket.
				ExtraArgs: terraform.ExtraArgs{Init: []string{"-backend=false"}},
				EnvVars:   map[string]string{"TF_PLUGIN_CACHE_DIR": cache},
			}
			terraform.InitAndValidateContext(t, t.Context(), opts)
		})
	}
}
