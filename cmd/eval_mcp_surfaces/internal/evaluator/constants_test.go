package evaluator

import "testing"

// TestCapabilityDiscoveryConstants_AreWired verifies the capability discovery
// preset and partition constants stay aligned with the public evaluator names.
func TestCapabilityDiscoveryConstants_AreWired(t *testing.T) {
	if presetDockerCapabilityDiscovery != "docker-capability-discovery" {
		t.Fatalf("presetDockerCapabilityDiscovery = %q", presetDockerCapabilityDiscovery)
	}
	if partitionCapabilityFallback != "capability-fallback" {
		t.Fatalf("partitionCapabilityFallback = %q", partitionCapabilityFallback)
	}
	if capabilityListTool != "gitlab_list_capabilities" {
		t.Fatalf("capabilityListTool = %q", capabilityListTool)
	}
	if completionTool != "gitlab_complete" {
		t.Fatalf("completionTool = %q", completionTool)
	}
}
