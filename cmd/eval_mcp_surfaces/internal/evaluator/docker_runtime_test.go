// docker_runtime_test.go covers the Docker Compose bootstrap used by the
// Docker-backed evaluation presets, including the gating logic that decides
// when the runtime should be auto-started.

package evaluator

import (
	"strings"
	"testing"
)

// TestShouldAutoStartDockerRuntime_RequiresDockerPresetAndGitLabBackend
// verifies that shouldAutoStartDockerRuntime only returns true when all three
// gates are satisfied: a Docker preset, the GitLab backend, and the
// --docker-auto-start flag.
//
// The test exercises the four combinations of these inputs and asserts the
// gate rejects mock backends, non-Docker presets, and the disabled flag.
// This protects evaluator runs from spawning Docker outside the controlled
// presets and from attempting a real-GitLab smoke when only mocks are
// configured.
func TestShouldAutoStartDockerRuntime_RequiresDockerPresetAndGitLabBackend(t *testing.T) {
	if !shouldAutoStartDockerRuntime(options{Preset: presetDockerRead, Backend: backendGitLab, DockerAutoStart: true}) {
		t.Fatal("shouldAutoStartDockerRuntime(docker gitlab) = false, want true")
	}
	if shouldAutoStartDockerRuntime(options{Preset: presetDockerRead, Backend: backendMock, DockerAutoStart: true}) {
		t.Fatal("shouldAutoStartDockerRuntime(mock backend) = true, want false")
	}
	if shouldAutoStartDockerRuntime(options{Preset: presetSchemaEnterprise, Backend: backendGitLab, DockerAutoStart: true}) {
		t.Fatal("shouldAutoStartDockerRuntime(non-docker preset) = true, want false")
	}
	if shouldAutoStartDockerRuntime(options{Preset: presetDockerRead, Backend: backendGitLab}) {
		t.Fatal("shouldAutoStartDockerRuntime(disabled) = true, want false")
	}
}

// TestDockerComposeCommand_UsesDefaultsAndOverrides verifies that
// dockerComposeCommand returns the documented defaults when no overrides are
// supplied and honors --docker-compose / --docker-compose-file overrides
// when they are.
//
// The test asserts the default invocation produces "docker compose -f
// test/e2e/docker-compose.yml" and that podman-based overrides are split
// correctly into a command plus arguments. This protects the auto-start
// bootstrap from launching against the wrong Compose file or with an
// unexpected command.
func TestDockerComposeCommand_UsesDefaultsAndOverrides(t *testing.T) {
	command, args := dockerComposeCommand(options{})
	if command != "docker" || strings.Join(args, " ") != "compose -f test/e2e/docker-compose.yml" {
		t.Fatalf("dockerComposeCommand(default) = %q %v", command, args)
	}

	command, args = dockerComposeCommand(options{DockerCompose: "podman compose", DockerComposeFile: "custom.yml"})
	if command != "podman" || strings.Join(args, " ") != "compose -f custom.yml" {
		t.Fatalf("dockerComposeCommand(override) = %q %v", command, args)
	}
}

// TestDockerRuntimeEnv_EnterpriseImageDefault verifies that
// dockerRuntimeEnv emits the Enterprise GitLab image and related env entries
// when the Enterprise preset and edition are selected.
//
// The test clears the GITLAB_IMAGE and EVAL_DOCKER_GITLAB_IMAGE environment
// variables, runs dockerRuntimeEnv for the docker-enterprise-read preset, and
// asserts the joined env output contains GITLAB_TIER=ultimate (the evaluated
// server reads GITLAB_TIER now), GITLAB_IMAGE=gitlab/gitlab-ee:latest, and the
// documented Docker Compose file. This protects Enterprise presets from
// regressing to the CE image.
func TestDockerRuntimeEnv_EnterpriseImageDefault(t *testing.T) {
	t.Setenv("GITLAB_IMAGE", "")
	t.Setenv("EVAL_DOCKER_GITLAB_IMAGE", "")
	env := strings.Join(dockerRuntimeEnv(options{Preset: presetDockerEnterpriseRead, Edition: editionEnterprise}), "\n")
	for _, want := range []string{"GITLAB_TIER=ultimate", "GITLAB_IMAGE=gitlab/gitlab-ee:latest", "E2E_DOCKER_COMPOSE_FILE=test/e2e/docker-compose.yml"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(env, want) {
				t.Fatalf("dockerRuntimeEnv() = %q, want %q", env, want)
			}
		})
	}
}
