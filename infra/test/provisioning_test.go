//go:build provisioning

package test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/gruntwork-io/terratest/modules/retry"
	"github.com/gruntwork-io/terratest/modules/shell"
	"github.com/gruntwork-io/terratest/modules/terraform"
	test_structure "github.com/gruntwork-io/terratest/modules/test-structure"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

// Provisioning tier: applies a real cluster (billable, ~20-25 minutes), asserts
// on it, and always destroys it. Run by hand with admin rights, never in CI.
//
// Stages let you iterate without paying for a new cluster each time:
//   SKIP_teardown=true go test ...   keep the cluster after the run
//   SKIP_setup=true    go test ...   reuse the kept cluster (and its saved options)
//   SKIP_validate=true go test ...   just build or destroy

// Manifests applied with kubectl into throwaway namespaces.
// The server comes from Docker Hub; the client from Quay.
const networkPolicyFixture = `
apiVersion: v1
kind: Pod
metadata:
  name: web
  labels: {app: web}
spec:
  containers:
    - name: nginx
      image: docker.io/library/nginx:1.31-alpine
---
apiVersion: v1
kind: Service
metadata:
  name: web
spec:
  selector: {app: web}
  ports:
    - port: 80
---
apiVersion: v1
kind: Pod
metadata:
  name: client
spec:
  containers:
    - name: curl
      image: quay.io/curl/curl:8.23.0
      command: ["sleep", "3600"]
`

const natProbe = `
apiVersion: v1
kind: Pod
metadata:
  name: nat-probe
spec:
  containers:
    - name: curl
      image: quay.io/curl/curl:8.23.0
      command: ["sleep", "3600"]
`

const denyAllIngress = `
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: default-deny-ingress
spec:
  podSelector: {}
  policyTypes: [Ingress]
`

// kubeconfigFor writes credentials for the fixture's cluster into a temporary
// kubeconfig, so the test never touches the developer's own ~/.kube/config.
// Needs gcloud and gke-gcloud-auth-plugin on PATH.
func kubeconfigFor(t *testing.T, project string, opts *terraform.Options) *k8s.KubectlOptions {
	t.Helper()
	ctx := t.Context()
	cluster := terraform.OutputContext(t, ctx, opts, "cluster_name")
	zone := terraform.OutputContext(t, ctx, opts, "zone")

	kubeconfig := filepath.Join(t.TempDir(), "kubeconfig")
	shell.RunCommandContext(t, ctx, &shell.Command{
		Command: "gcloud",
		Args:    []string{"container", "clusters", "get-credentials", cluster, "--zone", zone, "--project", project},
		Env:     map[string]string{"KUBECONFIG": kubeconfig},
	})
	return k8s.NewKubectlOptions("", kubeconfig, "default")
}

func TestGKEStackProvisioning(t *testing.T) {
	project := projectID(t)

	// Always destroy, even if setup or an assertion fails. Deferred stages run
	// when the test function exits, including after t.Fatal.
	defer test_structure.RunTestStage(t, "teardown", func() {
		opts := test_structure.LoadTerraformOptions(t, gkeStackDir)
		terraform.DestroyContext(t, context.Background(), opts)
	})

	test_structure.RunTestStage(t, "setup", func() {
		opts := fixtureOptions(project, uniqueSuffix(), gkeStackDir, []string{myCIDR(t)})
		// Saved before apply, so teardown can find the suffix even if apply fails halfway.
		test_structure.SaveTerraformOptions(t, gkeStackDir, opts)
		terraform.InitAndApplyContext(t, t.Context(), opts)
	})

	test_structure.RunTestStage(t, "validate", func() {
		opts := test_structure.LoadTerraformOptions(t, gkeStackDir)
		kopts := kubeconfigFor(t, project, opts)

		assertSpotNodesReady(t, kopts)
		assertNoExternalIPs(t, kopts)
		assertImagePullViaNAT(t, kopts)
		assertNetworkPolicyEnforced(t, kopts)

		// Registry exists.
		repo := terraform.OutputContext(t, t.Context(), opts, "repository_id")
		shell.RunCommandContext(t, t.Context(), &shell.Command{
			Command: "gcloud",
			Args:    []string{"artifacts", "repositories", "describe", repo, "--location", asiaSouthOne, "--project", project},
		})
	})
}

// At least 2 nodes Ready within 10 minutes, all of them spot.
func assertSpotNodesReady(t *testing.T, kopts *k8s.KubectlOptions) {
	t.Helper()
	retry.DoWithRetryContext(t, t.Context(), "wait for 2 ready nodes", 60, 10*time.Second, func() (string, error) {
		nodes, err := k8s.GetReadyNodesContextE(t, t.Context(), kopts)
		if err != nil {
			return "", err
		}
		if len(nodes) < 2 {
			return "", fmt.Errorf("only %d ready nodes", len(nodes))
		}
		return "", nil
	})

	for _, node := range k8s.GetNodesContext(t, t.Context(), kopts) {
		require.Equal(t, "true", node.Labels["cloud.google.com/gke-spot"], "node %s is not a spot node", node.Name)
	}
}

// Private nodes: nothing has an external address.
func assertNoExternalIPs(t *testing.T, kopts *k8s.KubectlOptions) {
	t.Helper()
	for _, node := range k8s.GetNodesContext(t, t.Context(), kopts) {
		for _, addr := range node.Status.Addresses {
			require.NotEqual(t, corev1.NodeExternalIP, addr.Type, "node %s has external IP %s", node.Name, addr.Address)
		}
	}
}

// NAT works: a pod can pull from Quay, which is not reachable through Google's
// private access. (Docker Hub images can come through Google's mirror, which
// would hide a missing NAT, so Quay is the stricter probe.) Without NAT this
// pod sits in ImagePullBackOff and WaitUntilPodAvailable fails.
func assertImagePullViaNAT(t *testing.T, kopts *k8s.KubectlOptions) {
	t.Helper()
	ns := k8s.NewKubectlOptions("", kopts.ConfigPath, "nat-test")
	k8s.CreateNamespaceContext(t, t.Context(), kopts, "nat-test")
	defer k8s.DeleteNamespaceContext(t, context.Background(), kopts, "nat-test")

	k8s.KubectlApplyFromStringContext(t, t.Context(), ns, natProbe)
	k8s.WaitUntilPodAvailableContext(t, t.Context(), ns, "nat-probe", 30, 10*time.Second)
}

// Dataplane V2 enforces NetworkPolicy: curl works, then a default-deny policy
// makes it fail.
func assertNetworkPolicyEnforced(t *testing.T, kopts *k8s.KubectlOptions) {
	t.Helper()
	k8s.CreateNamespaceContext(t, t.Context(), kopts, "np-test")
	defer k8s.DeleteNamespaceContext(t, context.Background(), kopts, "np-test")
	ns := k8s.NewKubectlOptions("", kopts.ConfigPath, "np-test")

	k8s.KubectlApplyFromStringContext(t, t.Context(), ns, networkPolicyFixture)
	k8s.WaitUntilPodAvailableContext(t, t.Context(), ns, "web", 30, 10*time.Second)
	k8s.WaitUntilPodAvailableContext(t, t.Context(), ns, "client", 30, 10*time.Second)

	curl := func() (string, error) {
		return k8s.RunKubectlAndGetOutputContextE(t, t.Context(), ns,
			"exec", "client", "--", "curl", "-sS", "-m", "3", "-o", "/dev/null", "http://web")
	}

	// Before the policy: allowed.
	retry.DoWithRetryContext(t, t.Context(), "curl before policy", 10, 3*time.Second, curl)

	// After the policy: must start failing within 30 seconds.
	k8s.KubectlApplyFromStringContext(t, t.Context(), ns, denyAllIngress)
	retry.DoWithRetryContext(t, t.Context(), "curl is blocked", 6, 5*time.Second, func() (string, error) {
		if _, err := curl(); err == nil {
			return "", &stillReachableError{}
		}
		return "blocked", nil
	})
}

type stillReachableError struct{}

func (*stillReachableError) Error() string { return "web is still reachable" }
