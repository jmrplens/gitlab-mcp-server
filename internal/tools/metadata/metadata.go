// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package metadata

import (
	"context"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// GetInput is the input (no params).
type GetInput struct{}

// KASInfo represents the KAS (Kubernetes Agent Server) metadata.
type KASInfo struct {
	Enabled             bool   `json:"enabled"`
	ExternalURL         string `json:"external_url"`
	ExternalK8SProxyURL string `json:"external_k8s_proxy_url"`
	Version             string `json:"version"`
}

// GetOutput is the output for metadata.
type GetOutput struct {
	toolutil.HintableOutput
	Version    string  `json:"version"`
	Revision   string  `json:"revision"`
	KAS        KASInfo `json:"kas"`
	Enterprise bool    `json:"enterprise"`
	// Plan is the plan of the license the instance runs under (free,
	// premium or ultimate), which lib/api/entities/metadata.rb sends from
	// GitLab 19.6 and client-go's Metadata does not model, read from the
	// captured response (ADR-0021). GitLab sends null where subscriptions
	// are scoped to each namespace rather than to the instance, as on
	// GitLab.com, and an older instance sends no key; both are left out.
	Plan *string `json:"plan,omitempty" jsonschema:"Plan of the license the instance runs under: free, premium or ultimate. Absent on GitLab.com, where plans belong to namespaces, and before GitLab 19.6"`
}

// Get retrieves GitLab instance metadata via the GitLab Metadata API
// (GET /metadata). Useful for discovering the running GitLab version,
// the KAS (Kubernetes Agent Server) configuration, the Enterprise flag and,
// from GitLab 19.6, the plan the instance is licensed for.
func Get(ctx context.Context, client *gitlabclient.Client, _ GetInput) (GetOutput, error) {
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	meta, _, err := client.GL().Metadata.GetMetadata(gl.WithContext(ctx))
	if err != nil {
		return GetOutput{}, toolutil.WrapErrWithStatusHint("get_metadata", err, http.StatusForbidden, "verify your token has read_api scope")
	}
	extra, err := toolutil.CapturedMetadata(captured)
	if err != nil {
		return GetOutput{}, toolutil.WrapErr("get_metadata", err)
	}
	return GetOutput{
		Plan:     extra.Plan,
		Version:  meta.Version,
		Revision: meta.Revision,
		KAS: KASInfo{
			Enabled:             meta.KAS.Enabled,
			ExternalURL:         meta.KAS.ExternalURL,
			ExternalK8SProxyURL: meta.KAS.ExternalK8SProxyURL,
			Version:             meta.KAS.Version,
		},
		Enterprise: meta.Enterprise,
	}, nil
}
