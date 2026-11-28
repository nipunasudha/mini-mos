//go:build plan

package test

import (
	"testing"

	"github.com/gruntwork-io/terratest/modules/terraform"
	"github.com/stretchr/testify/require"
)

// Plan tier: runs `terraform plan` against real GCP (needs ADC) but creates
// nothing. It asserts on the planned values, so a wrong setting fails here in a
// minute instead of after a 20 minute apply.

// planned returns the planned attributes of one resource.
func planned(t *testing.T, plan *terraform.PlanStruct, address string) map[string]any {
	t.Helper()
	rc, ok := plan.ResourceChangesMap[address]
	require.True(t, ok, "resource %s is not in the plan", address)
	after, ok := rc.Change.After.(map[string]any)
	require.True(t, ok, "resource %s has no planned values", address)
	return after
}

// block returns the first element of a nested block (Terraform JSON renders
// blocks as lists).
func block(t *testing.T, attrs map[string]any, name string) map[string]any {
	t.Helper()
	list, ok := attrs[name].([]any)
	require.True(t, ok && len(list) > 0, "block %s is missing", name)
	first, ok := list[0].(map[string]any)
	require.True(t, ok, "block %s is not an object", name)
	return first
}

func TestGKEStackPlan(t *testing.T) {
	project := projectID(t)
	// TEST-NET-3 (RFC 5737): a valid CIDR that is never a real address.
	opts := fixtureOptions(project, "plan01", gkeStackDir, []string{"203.0.113.0/24"})

	plan := terraform.InitAndPlanAndShowWithStructNoLogTempPlanFileContext(t, t.Context(), opts)

	cluster := planned(t, plan, "module.gke.google_container_cluster.this")
	pool := planned(t, plan, "module.gke.google_container_node_pool.spot[0]")

	t.Run("spot pool autoscales 2 to 4", func(t *testing.T) {
		require.Equal(t, true, block(t, pool, "node_config")["spot"])
		scale := block(t, pool, "autoscaling")
		require.EqualValues(t, 2, scale["min_node_count"])
		require.EqualValues(t, 4, scale["max_node_count"])
	})

	t.Run("private nodes with restricted public endpoint", func(t *testing.T) {
		require.Equal(t, true, block(t, cluster, "private_cluster_config")["enable_private_nodes"])
		require.NotEmpty(t, block(t, cluster, "master_authorized_networks_config")["cidr_blocks"])
	})

	t.Run("workload identity, dataplane v2, no deletion protection", func(t *testing.T) {
		require.Equal(t, project+".svc.id.goog", block(t, cluster, "workload_identity_config")["workload_pool"])
		require.Equal(t, "ADVANCED_DATAPATH", cluster["datapath_provider"])
		require.Equal(t, false, cluster["deletion_protection"])
	})

	t.Run("WIF provider is limited to the repo", func(t *testing.T) {
		provider := planned(t, plan, "module.registry.google_iam_workload_identity_pool_provider.github")
		require.Contains(t, provider["attribute_condition"], githubRepo)
	})

	t.Run("registry format is DOCKER", func(t *testing.T) {
		repo := planned(t, plan, "module.registry.google_artifact_registry_repository.this")
		require.Equal(t, "DOCKER", repo["format"])
	})
}
