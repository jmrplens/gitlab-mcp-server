package evaluator

import (
	"strings"
	"testing"
)

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

func TestDockerRuntimeEnv_EnterpriseImageDefault(t *testing.T) {
	t.Setenv("GITLAB_IMAGE", "")
	t.Setenv("EVAL_DOCKER_GITLAB_IMAGE", "")
	env := strings.Join(dockerRuntimeEnv(options{Preset: presetDockerEnterpriseRead, Edition: editionEnterprise}), "\n")
	for _, want := range []string{"GITLAB_ENTERPRISE=true", "GITLAB_IMAGE=gitlab/gitlab-ee:latest", "E2E_DOCKER_COMPOSE_FILE=test/e2e/docker-compose.yml"} {
		if !strings.Contains(env, want) {
			t.Fatalf("dockerRuntimeEnv() = %q, want %q", env, want)
		}
	}
}
