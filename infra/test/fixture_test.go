//go:build plan || provisioning

package test

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/gruntwork-io/terratest/modules/random"
	"github.com/gruntwork-io/terratest/modules/terraform"
	"github.com/stretchr/testify/require"
)

// Helpers shared by the plan and provisioning tiers.

const (
	gkeStackDir  = "fixtures/gke-stack"
	platformDir  = "fixtures/platform"
	githubRepo   = "nipunasudha/mini-mos"
	asiaSouthOne = "asia-south1"
)

// projectID reads the GCP project from the environment. It is never committed.
func projectID(t *testing.T) string {
	t.Helper()
	for _, key := range []string{"GOOGLE_PROJECT", "TF_VAR_project_id"} {
		if v := os.Getenv(key); v != "" {
			return v
		}
	}
	require.FailNow(t, "set GOOGLE_PROJECT (or TF_VAR_project_id) to the GCP project to test in")
	return ""
}

// uniqueSuffix is random.UniqueId lower-cased: it returns mixed case, but GCP
// resource names must be lower-case.
func uniqueSuffix() string {
	return strings.ToLower(random.UniqueId())
}

// fixtureOptions builds the options for a fixture directory.
func fixtureOptions(project, suffix, dir string, cidrs []string) *terraform.Options {
	return &terraform.Options{
		TerraformDir: dir,
		NoColor:      true,
		Vars: map[string]any{
			"project_id":              project,
			"suffix":                  suffix,
			"master_authorized_cidrs": cidrs,
		},
	}
}

// myCIDR is the address this machine reaches the internet from. It must be in
// master_authorized_networks, or the test (and Terraform's helm provider) could
// not talk to the cluster. Override with MASTER_AUTHORIZED_CIDR.
func myCIDR(t *testing.T) string {
	t.Helper()
	if v := os.Getenv("MASTER_AUTHORIZED_CIDR"); v != "" {
		return v
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://checkip.amazonaws.com", nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err, "could not detect public IP; set MASTER_AUTHORIZED_CIDR=<ip>/32")
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return strings.TrimSpace(string(body)) + "/32"
}
