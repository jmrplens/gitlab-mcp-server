package attestations

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for build attestation actions. The list
// and download routes are projected into the dynamic, meta, individual, and
// audit surfaces by the action catalog (ADR-0004). Each spec carries
// action-specific discovery metadata (Usage, supply-chain natural-language
// Aliases, canonical RelatedActions, ParameterGuidance, and an individual-tool
// "Returns: … See also: …" Description) per the 1:1 audit R-META requirement.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		attestationReadSpec("list", toolutil.RouteAction(client, List), "gitlab_list_attestations"),
		attestationReadSpec("download", toolutil.RouteAction(client, Download), "gitlab_download_attestation"),
	}
}

func attestationReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	meta := attestationActionMeta[individualTool]

	return toolutil.NewReadActionSpec(name, route, toolutil.ActionSpecOptions{
		Aliases:           append([]string{individualTool}, meta.aliases...),
		Tags:              []string{"security", "attestation"},
		Usage:             meta.usage,
		RelatedActions:    meta.related,
		ParameterGuidance: meta.guidance,
		OpenWorld:         true,
		Edition:           "premium",
		OwnerPackage:      "attestations",
		IndividualTool: toolutil.IndividualToolSpec{
			Name:        individualTool,
			Title:       toolutil.TitleFromName(individualTool),
			Description: meta.description,
		},
	})
}

// attestationActionMetaEntry is the discovery metadata for one attestation
// action (1:1 audit R-META).
type attestationActionMetaEntry struct {
	usage       string
	aliases     []string
	related     []string
	guidance    map[string]toolutil.ParameterGuidance
	description string
}

// attestationActionMeta maps each individual attestation tool to its non-generic
// discovery metadata: action-specific Usage, distinctive supply-chain Aliases,
// canonical RelatedActions, ParameterGuidance, and the "Returns: … See also: …"
// individual-tool description.
var attestationActionMeta = map[string]attestationActionMetaEntry{
	"gitlab_list_attestations": {
		usage: "List SLSA build provenance attestations for a project artifact identified by its subject digest. Use this to discover which signed attestations exist for a built image or package before downloading one, and to obtain each attestation's IID, predicate type, and status. Requires an Ultimate license; provide the project plus the OCI-style subject digest of the artifact.",
		aliases: []string{
			"list build attestations",
			"list slsa provenance",
			"list supply chain attestations",
			"find signed provenance for artifact",
			"show attestations for image digest",
		},
		related: []string{"attestation.download", "package.list", "project.get"},
		guidance: map[string]toolutil.ParameterGuidance{
			"project_id": {
				SemanticRole:     "scope_project",
				ValueSource:      "Project ID or full namespace path that owns the attested artifact.",
				ExampleBinding:   `params.project_id:"my-org/platform"`,
				CommonConfusions: []string{"Use the project that built and attested the artifact, not the registry or group path."},
			},
			"subject_digest": {
				SemanticRole:     "artifact_digest",
				ValueSource:      "OCI-style content digest of the attested artifact (algorithm-prefixed hash).",
				ExampleBinding:   `params.subject_digest:"sha256:abc123"`,
				CommonConfusions: []string{"Use the artifact's content digest (for example sha256:...), not a Git commit SHA, package version, or attestation IID."},
			},
		},
		description: "List SLSA build provenance attestations for a project artifact by subject digest (Ultimate). Returns: each attestation's id, iid, project_id, build_id, status, predicate_kind, predicate_type, subject_digest, download_url, and created/updated/expire timestamps. See also: gitlab_download_attestation, gitlab_list_packages, gitlab_get_project.",
	},
	"gitlab_download_attestation": {
		usage: "Download the raw in-toto attestation bundle for a single attestation by its project-scoped IID. Use this after gitlab_list_attestations identifies the attestation you want to verify; the response carries the base64-encoded bundle content and its byte size. Requires an Ultimate license.",
		aliases: []string{
			"download build attestation",
			"download slsa provenance bundle",
			"fetch in-toto attestation",
			"get signed provenance content",
			"retrieve attestation bundle",
		},
		related: []string{"attestation.list", "package.list", "project.get"},
		guidance: map[string]toolutil.ParameterGuidance{
			"project_id": {
				SemanticRole:     "scope_project",
				ValueSource:      "Project ID or full namespace path that owns the attestation.",
				ExampleBinding:   `params.project_id:"my-org/platform"`,
				CommonConfusions: []string{"Use the project that owns the attestation, not the registry or group path."},
			},
			"attestation_iid": {
				SemanticRole:     "attestation_iid",
				ValueSource:      "Project-scoped attestation IID returned by gitlab_list_attestations.",
				ExampleBinding:   "params.attestation_iid:1",
				CommonConfusions: []string{"attestation_iid is the per-project IID from the list action, not the global id field and not the artifact subject digest."},
			},
		},
		description: "Download the raw in-toto attestation bundle for one attestation by IID (Ultimate). Returns: the attestation_iid, the bundle size in bytes, and the base64-encoded content_base64 bundle payload. See also: gitlab_list_attestations, gitlab_list_packages, gitlab_get_project.",
	},
}
