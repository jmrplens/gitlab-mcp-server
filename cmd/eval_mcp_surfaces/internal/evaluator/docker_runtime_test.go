// docker_runtime_test.go covers the Docker Compose bootstrap used by the
// Docker-backed evaluation presets, including the gating logic that decides
// when the runtime should be auto-started.

package evaluator

import (
	"runtime"
	"strings"
	"testing"
	"time"
)

// exitingCommand is a command that exits with status: the POSIX true and
// false on unix, cmd.exe's exit built-in on Windows, where neither exists.
func exitingCommand(status int) (string, []string) {
	if runtime.GOOS == "windows" {
		return "cmd", []string{"/c", "exit", map[bool]string{true: "1", false: "0"}[status != 0]}
	}
	if status != 0 {
		return "false", nil
	}
	return "true", nil
}

// sleepingCommand is a command that outlives any short timeout without
// spawning a child of its own: a grandchild holding the output pipe would
// keep the wait going after the process under test had been killed.
func sleepingCommand() (string, []string) {
	if runtime.GOOS == "windows" {
		return "powershell", []string{"-NoProfile", "-NonInteractive", "-Command", "Start-Sleep -Seconds 5"}
	}
	return "sleep", []string{"5"}
}

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

// TestEnsureDockerRuntimeIfNeeded_AutoStartDisabled_ReturnsNil verifies no
// Docker command runs when auto-start is off.
func TestEnsureDockerRuntimeIfNeeded_AutoStartDisabled_ReturnsNil(t *testing.T) {
	if err := ensureDockerRuntimeIfNeeded(t.Context(), options{Preset: presetDockerRead, Backend: backendGitLab}); err != nil {
		t.Fatalf("ensureDockerRuntimeIfNeeded() error = %v, want nil", err)
	}
}

// TestEnsureDockerRuntimeIfNeeded_ComposeFails_ReturnsDockerUpError verifies
// the first compose step failing aborts the runtime bootstrap with the step
// name. The compose command is the `false` binary, so nothing Docker-related
// is touched.
func TestEnsureDockerRuntimeIfNeeded_ComposeFails_ReturnsDockerUpError(t *testing.T) {
	t.Setenv("DOCKER_COMPOSE", "")
	opts := options{Preset: presetDockerRead, Backend: backendGitLab, DockerAutoStart: true, DockerCompose: "false", DockerWaitTimeout: time.Second}
	err := ensureDockerRuntimeIfNeeded(t.Context(), opts)
	if err == nil || !strings.Contains(err.Error(), "docker-up") {
		t.Fatalf("ensureDockerRuntimeIfNeeded() error = %v, want docker-up failure", err)
	}
}

// TestDockerGitLabURL_PrefersFlagThenEnvThenDefault verifies the Docker
// GitLab URL resolution order.
func TestDockerGitLabURL_PrefersFlagThenEnvThenDefault(t *testing.T) {
	cases := []struct {
		name string
		flag string
		env  string
		want string
	}{
		{name: "flag", flag: "http://flag:1", env: "http://env:2", want: "http://flag:1"},
		{name: "env", env: "http://env:2", want: "http://env:2"},
		{name: "default", want: defaultDockerGitLabURL},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("EVAL_DOCKER_GITLAB_URL", tc.env)
			if got := dockerGitLabURL(options{DockerGitLabURL: tc.flag}); got != tc.want {
				t.Fatalf("dockerGitLabURL() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestEnvBool_ParsesTruthyValues verifies the accepted truthy spellings and
// that everything else is false.
func TestEnvBool_ParsesTruthyValues(t *testing.T) {
	cases := []struct {
		value string
		want  bool
	}{
		{value: "1", want: true},
		{value: " TRUE ", want: true},
		{value: "yes", want: true},
		{value: "y", want: true},
		{value: "on", want: true},
		{value: "0", want: false},
		{value: "", want: false},
		{value: "maybe", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.value, func(t *testing.T) {
			t.Setenv("EVAL_TEST_ENV_BOOL", tc.value)
			if got := envBool("EVAL_TEST_ENV_BOOL"); got != tc.want {
				t.Fatalf("envBool(%q) = %t, want %t", tc.value, got, tc.want)
			}
		})
	}
}

// TestDockerEnterpriseRuntime_DetectsEditionPresetAndEnvironment verifies the
// enterprise runtime is selected from the edition flag, an enterprise preset,
// GITLAB_TIER, the legacy GITLAB_ENTERPRISE toggle, or an EE image name.
func TestDockerEnterpriseRuntime_DetectsEditionPresetAndEnvironment(t *testing.T) {
	cases := []struct {
		name string
		opts options
		env  map[string]string
		want bool
	}{
		{name: "edition flag", opts: options{Edition: editionEnterprise}, want: true},
		{name: "enterprise preset", opts: options{Preset: presetDockerEnterpriseRead}, want: true},
		{name: "tier ultimate", env: map[string]string{"GITLAB_TIER": "ultimate"}, want: true},
		{name: "tier free", env: map[string]string{"GITLAB_TIER": "free"}, want: false},
		{name: "legacy enterprise toggle", env: map[string]string{"GITLAB_ENTERPRISE": "true"}, want: true},
		{name: "ee image", env: map[string]string{"GITLAB_IMAGE": "gitlab/gitlab-ee:latest"}, want: true},
		{name: "eval ee image", env: map[string]string{"EVAL_DOCKER_GITLAB_IMAGE": "gitlab/gitlab-ee:16"}, want: true},
		{name: "ce default", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, key := range []string{"GITLAB_TIER", "GITLAB_ENTERPRISE", "GITLAB_IMAGE", "EVAL_DOCKER_GITLAB_IMAGE"} {
				t.Setenv(key, tc.env[key])
			}
			if got := dockerEnterpriseRuntime(tc.opts); got != tc.want {
				t.Fatalf("dockerEnterpriseRuntime() = %t, want %t", got, tc.want)
			}
		})
	}
}

// TestRunDockerRuntimeCommand_ReportsFailureAndTimeout verifies a failing
// command is reported under its step name and a command that outlives its
// timeout is reported as timed out. The commands are the platform's own
// plain utilities, since none of true, false or sleep exists on Windows.
func TestRunDockerRuntimeCommand_ReportsFailureAndTimeout(t *testing.T) {
	failing, failingArgs := exitingCommand(1)
	succeeding, succeedingArgs := exitingCommand(0)
	sleeping, sleepingArgs := sleepingCommand()
	cases := []struct {
		name    string
		timeout time.Duration
		command string
		args    []string
		want    string
	}{
		{name: "exit status", timeout: 5 * time.Second, command: failing, args: failingArgs, want: "step-name: exit status 1"},
		{name: "timeout", timeout: 20 * time.Millisecond, command: sleeping, args: sleepingArgs, want: "step-name timed out after"},
		{name: "success", timeout: 5 * time.Second, command: succeeding, args: succeedingArgs},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := runDockerRuntimeCommand(t.Context(), tc.timeout, nil, "step-name", tc.command, tc.args...)
			if tc.want == "" {
				if err != nil {
					t.Fatalf("runDockerRuntimeCommand() error = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("runDockerRuntimeCommand() error = %v, want %q", err, tc.want)
			}
		})
	}
}
