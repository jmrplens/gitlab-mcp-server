package metadata

import (
	"context"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
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
}

// Get retrieves GitLab instance metadata.
func Get(ctx context.Context, client *gitlabclient.Client, _ GetInput) (GetOutput, error) {
	meta, _, err := client.GL().Metadata.GetMetadata(gl.WithContext(ctx))
	if err != nil {
		return GetOutput{}, toolutil.WrapErrWithStatusHint("get_metadata", err, http.StatusForbidden, "verify your token has read_api scope")
	}
	return GetOutput{
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
