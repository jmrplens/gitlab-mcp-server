// Package dbmigrations implements MCP tools for GitLab Database Migrations API.
package dbmigrations

import (
	"context"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// MarkInput is the input for marking a migration as successful.
type MarkInput struct {
	Version  int64  `json:"version" jsonschema:"Migration version number to mark as successful,required"`
	Database string `json:"database,omitempty" jsonschema:"Database name (optional, e.g. main or ci)"`
}

// MarkOutput is the output for marking a migration as successful.
type MarkOutput struct {
	toolutil.HintableOutput
	Status  string `json:"status"`
	Version int64  `json:"version"`
}

// Mark marks a pending database migration as successfully executed.
func Mark(ctx context.Context, client *gitlabclient.Client, input MarkInput) (MarkOutput, error) {
	if input.Version <= 0 {
		return MarkOutput{}, toolutil.ErrRequiredInt64("mark_migration", "version")
	}
	opts := &gl.MarkMigrationAsSuccessfulOptions{
		Database: input.Database,
	}

	_, err := client.GL().DatabaseMigrations.MarkMigrationAsSuccessful(input.Version, opts, gl.WithContext(ctx))
	if err != nil {
		return MarkOutput{}, toolutil.WrapErrWithStatusHint("mark_migration", err, http.StatusForbidden, "database migrations require administrator access")
	}
	return MarkOutput{
		Status:  "marked",
		Version: input.Version,
	}, nil
}
