package evaluator

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func ensureDockerRuntimeIfNeeded(ctx context.Context, opts options) error {
	if !shouldAutoStartDockerRuntime(opts) {
		return nil
	}
	gitlabURL := dockerGitLabURL(opts)
	composeCommand, composeArgs := dockerComposeCommand(opts)
	commandEnv := dockerRuntimeEnv(opts)
	terminalPrintf("docker: ensuring GitLab fixture stack at %s\n", gitlabURL)
	if err := runDockerRuntimeCommand(ctx, 2*time.Minute, commandEnv, "docker-up", composeCommand, append(composeArgs, "up", "-d")...); err != nil {
		return err
	}
	if err := runDockerRuntimeCommand(ctx, opts.DockerWaitTimeout+30*time.Second, commandEnv, "wait-for-gitlab", "./test/e2e/scripts/wait-for-gitlab.sh", gitlabURL, strconv.Itoa(int(opts.DockerWaitTimeout.Seconds()))); err != nil {
		return err
	}
	if err := runDockerRuntimeCommand(ctx, 8*time.Minute, commandEnv, "setup-gitlab", "./test/e2e/scripts/setup-gitlab.sh", gitlabURL); err != nil {
		return err
	}
	if err := runDockerRuntimeCommand(ctx, 8*time.Minute, commandEnv, "register-runner", "./test/e2e/scripts/register-runner.sh", gitlabURL); err != nil {
		return err
	}
	return nil
}

func shouldAutoStartDockerRuntime(opts options) bool {
	return opts.DockerAutoStart && isDockerPreset(opts.Preset) && normalizedBackend(opts.Backend) == backendGitLab
}

func isDockerPreset(preset string) bool {
	return strings.HasPrefix(strings.TrimSpace(preset), "docker-")
}

func dockerComposeCommand(opts options) (command string, args []string) {
	compose := strings.TrimSpace(firstNonEmpty(opts.DockerCompose, os.Getenv("DOCKER_COMPOSE")))
	parts := strings.Fields(compose)
	if len(parts) == 0 {
		parts = []string{"docker", "compose"}
	}
	args = append([]string{}, parts[1:]...)
	args = append(args, "-f", dockerComposeFile(opts))
	return parts[0], args
}

func dockerComposeFile(opts options) string {
	return firstNonEmpty(opts.DockerComposeFile, firstNonEmpty(os.Getenv("EVAL_DOCKER_COMPOSE_FILE"), defaultDockerComposeFile))
}

func dockerGitLabURL(opts options) string {
	return firstNonEmpty(opts.DockerGitLabURL, firstNonEmpty(os.Getenv("EVAL_DOCKER_GITLAB_URL"), defaultDockerGitLabURL))
}

func dockerRuntimeEnv(opts options) []string {
	enterprise := dockerEnterpriseRuntime(opts)
	env := []string{
		"E2E_DOCKER_COMPOSE_FILE=" + dockerComposeFile(opts),
		"GITLAB_ENTERPRISE=" + strconv.FormatBool(enterprise),
	}
	image := os.Getenv("EVAL_DOCKER_GITLAB_IMAGE")
	if image == "" && enterprise && os.Getenv("GITLAB_IMAGE") == "" {
		image = defaultDockerGitLabEEImage
	}
	if image != "" {
		env = append(env, "GITLAB_IMAGE="+image)
	}
	return env
}

func dockerEnterpriseRuntime(opts options) bool {
	if opts.Edition == editionEnterprise || strings.HasPrefix(opts.Preset, "docker-enterprise-") {
		return true
	}
	return envBool("GITLAB_ENTERPRISE") || strings.Contains(os.Getenv("GITLAB_IMAGE"), "gitlab-ee") || strings.Contains(os.Getenv("EVAL_DOCKER_GITLAB_IMAGE"), "gitlab-ee")
}

func envBool(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func runDockerRuntimeCommand(ctx context.Context, timeout time.Duration, env []string, name, command string, args ...string) error {
	commandCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	terminalLogPrintf("$ %s\n", strings.Join(append([]string{command}, args...), " "))
	cmd := exec.CommandContext(commandCtx, command, args...) // #nosec G204 -- evaluator Docker commands are explicit CLI/env developer inputs, executed without a shell.
	cmd.Env = append(os.Environ(), env...)
	output, err := cmd.CombinedOutput()
	if len(output) > 0 {
		terminalLogPrintf("%s", output)
	}
	if commandCtx.Err() != nil {
		return fmt.Errorf("%s timed out after %s: %w", name, timeout, commandCtx.Err())
	}
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}
