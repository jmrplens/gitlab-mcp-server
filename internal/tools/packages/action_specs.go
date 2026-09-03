package packages

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	actionPackageList     = "package.list"
	actionPackageFileList = "package.file_list"
	actionPackagePublish  = "package.publish"
	actionPackageDelete   = "package.delete"
	actionNameList        = "list"
	actionNameGroupList   = "group_list"
	actionNamePublishDir  = "publish_directory"
	schemaEnum            = "enum"
	schemaDescription     = "description"
	paramOrderBy          = "order_by"
)

// ActionSpecs returns canonical specs for Generic Package Registry
// actions exposed as MCP tools. The publish, download, list, file
// list, delete, file delete, publish-and-link, and publish-directory
// routes are projected into the dynamic, meta, individual, and audit
// surfaces by the action catalog (ADR-0004).
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		// gitlab_package_publish — publish a file to the Generic Package Registry.
		packageCreateSpec("publish", toolutil.RouteActionWithRequest(client, Publish), "gitlab_package_publish"),
		// gitlab_package_download — download a file from the Generic Package
		// Registry. Classified as mutating, not read-only: the read is of the
		// registry, but the write is to this machine's disk, and every
		// protective mode keys on that flag rather than on what the handler
		// does. Read-only removes it, safe mode previews it, and a client that
		// auto-approves readOnlyHint:true no longer auto-approves a file write.
		packageWriteSpec("download", toolutil.RouteActionWithRequest(client, Download), "gitlab_package_download"),
		// gitlab_package_list — list project packages with optional filters and ordering.
		packageReadSpec(actionNameList, toolutil.RouteAction(client, List), "gitlab_package_list"),
		// gitlab_list_group_packages — list packages across a group and its descendant projects.
		packageReadSpec(actionNameGroupList, toolutil.RouteAction(client, GroupList), "gitlab_list_group_packages"),
		// gitlab_package_file_list — list files within a single package.
		packageReadSpec("file_list", toolutil.RouteAction(client, FileList), "gitlab_package_file_list"),
		// gitlab_package_delete — delete a package (destructive).
		packageDeleteSpec("delete", toolutil.DestructiveActionWithRequest(client, deleteOutput), "gitlab_package_delete"),
		// gitlab_package_file_delete — delete a single file from a package (destructive).
		packageDeleteSpec("file_delete", toolutil.DestructiveActionWithRequest(client, fileDeleteOutput), "gitlab_package_file_delete"),
		// gitlab_package_publish_and_link — publish a file and link it to a release in one call.
		packageCreateSpec("publish_and_link", toolutil.RouteActionWithRequest(client, PublishAndLink), "gitlab_package_publish_and_link"),
		// gitlab_package_publish_directory — publish every file from a local directory.
		packageCreateSpec(actionNamePublishDir, toolutil.RouteActionWithRequest(client, PublishDirectory), "gitlab_package_publish_directory"),
	}
}

// deleteOutput adapts the package's [Delete] handler to the
// [toolutil.DestructiveAction] contract, returning a structured
// success result that names the deleted package.
func deleteOutput(ctx context.Context, req *mcp.CallToolRequest, client *gitlabclient.Client, input DeleteInput) (toolutil.DeleteOutput, error) {
	if err := Delete(ctx, req, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult(fmt.Sprintf("package %s from project %s", input.PackageID, input.ProjectID))
	return out, nil
}

// fileDeleteOutput adapts the package's [FileDelete] handler to the
// [toolutil.DestructiveAction] contract, returning a structured
// success result that names the deleted file.
func fileDeleteOutput(ctx context.Context, req *mcp.CallToolRequest, client *gitlabclient.Client, input FileDeleteInput) (toolutil.DeleteOutput, error) {
	if err := FileDelete(ctx, req, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult(fmt.Sprintf("file %s from package %s in project %s", input.PackageFileID, input.PackageID, input.ProjectID))
	return out, nil
}

// packageReadSpec builds a read-only [toolutil.ActionSpec] for a
// Generic Package Registry action using the package's default
// [packageOptions].
func packageReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, packageOptions(name, individualTool))
}

// packageWriteSpec builds a mutating, idempotent [toolutil.ActionSpec] for a
// Generic Package Registry action that writes to the local filesystem, using
// the package's default [packageOptions].
func packageWriteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, packageOptions(name, individualTool))
}

// packageCreateSpec builds a create-style [toolutil.ActionSpec] for a
// Generic Package Registry action using the package's default
// [packageOptions].
func packageCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, packageOptions(name, individualTool))
}

// packageDeleteSpec builds a destructive [toolutil.ActionSpec] for a
// Generic Package Registry action using the package's default
// [packageOptions].
func packageDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, packageOptions(name, individualTool))
}

// packageActionMeta bundles the discovery-metadata fields (action-specific
// Usage, natural-language Aliases, canonical RelatedActions cross-links, and
// the R-META "Returns: … See also: …" individual-tool Description) for a single
// Generic Package Registry action.
type packageActionMeta struct {
	usage       string
	aliases     []string
	related     []string
	description string
}

// packageActionMetadata provides per-action discovery metadata for each
// Generic Package Registry action so that no action falls back to a generic
// Usage, toolname-only Aliases, or empty RelatedActions (1:1 audit R-META).
var packageActionMetadata = map[string]packageActionMeta{
	"publish": {
		usage:       "Upload a single file as a Generic Package Registry asset. Provide project_id, package_name, package_version, and file_name plus the local file to stream. Optional status and select fields control visibility and processing. To upload ALL files from a local directory in one call, use publish_directory instead.",
		aliases:     []string{"upload generic package file", "publish package asset", "push file to package registry", "create generic package version"},
		related:     []string{"package.publish_and_link", "package.publish_directory", actionPackageList, actionPackageFileList},
		description: "Publish a single file to the Generic Package Registry. Returns: the published file's id, package id, name, size, checksums (md5/sha1/sha256), and download URL. See also: gitlab_package_publish_and_link, gitlab_package_publish_directory, gitlab_package_list.",
	},
	"download": {
		usage:       "Download one Generic Package Registry file to a local path. Provide project_id, package_name, package_version, file_name, and the output_path to write. The file is streamed and checksummed locally.",
		aliases:     []string{"download generic package file", "fetch package asset", "pull file from package registry", "retrieve package version file"},
		related:     []string{actionPackageFileList, actionPackageList, actionPackagePublish},
		description: "Download a single file from the Generic Package Registry to a local path. Returns: the local output path, byte size, and SHA256 checksum. See also: gitlab_package_file_list, gitlab_package_list.",
	},
	actionNameList: {
		// usage is set in packageOptions with extra ordering guidance.
		aliases:     []string{"list project packages", "browse package registry", "find published packages", "enumerate package versions"},
		related:     []string{actionPackageFileList, actionPackagePublish, actionPackageDelete},
		description: "List packages in a project with optional filters and ordering. Returns: matching packages with type, status, pipeline metadata, tags, _links, and pagination metadata. See also: gitlab_package_file_list, gitlab_package_publish, gitlab_package_delete.",
	},
	actionNameGroupList: {
		// usage is set in packageOptions with extra ordering guidance.
		aliases:     []string{"list group packages", "browse group package registry", "list packages across group projects", "enumerate packages in a group"},
		related:     []string{actionPackageList, actionPackageFileList, actionPackageDelete},
		description: "List packages across a group and its descendant projects with optional filters and ordering. Returns: matching packages with type, status, owning project id/path, pipeline metadata, tags, _links, and pagination metadata. See also: gitlab_package_list, gitlab_package_file_list, gitlab_package_delete.",
	},
	"file_list": {
		usage:       "List the individual files belonging to one package. Provide project_id and the package_id returned by package.list to enumerate every asset, its size, and checksums.",
		aliases:     []string{"list package files", "show package assets", "enumerate files in package", "browse package version files"},
		related:     []string{"package.download", "package.file_delete", actionPackageList},
		description: "List the files within a single package. Returns: package files with name, size, checksums, creation time, and pagination metadata. See also: gitlab_package_download, gitlab_package_file_delete, gitlab_package_list.",
	},
	"delete": {
		usage:       "Permanently delete an entire package and all of its files. Provide project_id and the package_id from package.list. This is irreversible and removes every version asset.",
		aliases:     []string{"delete package", "remove package from registry", "purge package version", "destroy published package"},
		related:     []string{actionPackageList, "package.file_delete", actionPackageFileList},
		description: "Delete a package permanently. Returns: a success confirmation naming the deleted package and project. See also: gitlab_package_list, gitlab_package_file_delete.",
	},
	"file_delete": {
		usage:       "Permanently delete a single file from a package while keeping the package itself. Provide project_id, package_id, and the package_file_id from package.file_list.",
		aliases:     []string{"delete package file", "remove one package asset", "purge single package file", "drop file from package"},
		related:     []string{actionPackageFileList, actionPackageDelete, actionPackageList},
		description: "Delete a single file from a package permanently. Returns: a success confirmation naming the deleted file, package, and project. See also: gitlab_package_file_list, gitlab_package_delete.",
	},
	"publish_and_link": {
		usage:       "Publish a Generic Package Registry file and attach it to a release as an asset link in one call. Provide the package coordinates plus the tag_name of the target release.",
		aliases:     []string{"publish package and link to release", "upload package asset and attach release link", "attach generic package to release", "publish file and create release asset"},
		related:     []string{actionPackagePublish, "package.publish_directory", "release.create", "release.link_create"},
		description: "Publish a file to the Generic Package Registry and link it to a release in one call. Returns: the published file metadata and the created release asset link. See also: gitlab_package_publish, gitlab_package_publish_directory, gitlab_release_get.",
	},
	actionNamePublishDir: {
		// usage, aliases, and related are set in packageOptions with extra
		// glob guidance and tags.
		description: "Publish every regular file from a local directory to the Generic Package Registry. Returns: total files, total bytes, per-file results, and any per-file errors. See also: gitlab_package_publish, gitlab_package_publish_and_link, gitlab_package_list.",
	},
}

// packageOptions returns the base [toolutil.ActionSpecOptions] for a
// Generic Package Registry action and customizes the Usage,
// ParameterGuidance, and InputSchemaOverrides for the list and
// publish_directory individual tools.
func packageOptions(actionName, individualTool string) toolutil.ActionSpecOptions {
	options := toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute packages domain action.", Tags: []string{"package"},
		OpenWorld:      true,
		OwnerPackage:   "packages",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
	if meta, ok := packageActionMetadata[actionName]; ok {
		if meta.usage != "" {
			options.Usage = meta.usage
		}
		if len(meta.aliases) > 0 {
			options.Aliases = append([]string{individualTool}, meta.aliases...)
		}
		if len(meta.related) > 0 {
			options.RelatedActions = meta.related
		}
		if meta.description != "" {
			options.IndividualTool.Description = meta.description
		}
	}
	if actionName == actionNameList {
		options.Usage = "List package registry packages. If ordering is requested, use order_by with one of created_at, name, version, or type. Do not use updated_at, released_at, or downloaded_at."
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			paramOrderBy: {
				SemanticRole: "package_list_sort_field",
				ValueSource:  "Use only GitLab Package Registry ordering fields accepted by the packages API.",
				CommonConfusions: []string{
					"Do not use updated_at, released_at, downloaded_at, last_downloaded_at, or id as order_by values.",
				},
			},
		}
		options.InputSchemaOverrides = []toolutil.InputSchemaOverride{
			toolutil.SchemaPropertyOverride(paramOrderBy, map[string]any{
				schemaEnum:        []any{"created_at", "name", "version", "type"},
				schemaDescription: "Order by package registry field: created_at, name, version, or type.",
			}),
			toolutil.SchemaPropertyOverride("sort", map[string]any{schemaEnum: []any{"asc", "desc"}}),
			toolutil.SchemaPropertyOverride("status", map[string]any{
				schemaEnum:        []any{"default", "hidden", "processing", "error", "pending_destruction", "deprecated"},
				schemaDescription: "Filter by status: default, hidden, processing, error, pending_destruction, or deprecated.",
			}),
			toolutil.SchemaPropertyOverride("package_type", map[string]any{
				schemaEnum:        []any{"composer", "conan", "generic", "golang", "helm", "maven", "npm", "nuget", "pypi", "terraform_module"},
				schemaDescription: "Filter by package type: composer, conan, generic, golang, helm, maven, npm, nuget, pypi, or terraform_module.",
			}),
		}
	}
	if actionName == actionNameGroupList {
		options.Usage = "List package registry packages across a group and its descendant projects. Set exclude_subgroups=true to limit results to the group's direct projects. If ordering is requested, use order_by with one of created_at, name, version, type, or project_path."
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			paramOrderBy: {
				SemanticRole: "group_package_list_sort_field",
				ValueSource:  "Use only GitLab group Package Registry ordering fields accepted by the group packages API.",
				CommonConfusions: []string{
					"Do not use updated_at, released_at, downloaded_at, last_downloaded_at, or id as order_by values.",
				},
			},
		}
		options.InputSchemaOverrides = []toolutil.InputSchemaOverride{
			toolutil.SchemaPropertyOverride(paramOrderBy, map[string]any{
				schemaEnum:        []any{"created_at", "name", "version", "type", "project_path"},
				schemaDescription: "Order by group package registry field: created_at, name, version, type, or project_path.",
			}),
			toolutil.SchemaPropertyOverride("sort", map[string]any{schemaEnum: []any{"asc", "desc"}}),
			toolutil.SchemaPropertyOverride("status", map[string]any{
				schemaEnum:        []any{"default", "hidden", "processing", "error", "pending_destruction", "deprecated"},
				schemaDescription: "Filter by status: default, hidden, processing, error, pending_destruction, or deprecated.",
			}),
			toolutil.SchemaPropertyOverride("package_type", map[string]any{
				schemaEnum:        []any{"composer", "conan", "generic", "golang", "helm", "maven", "npm", "nuget", "pypi", "terraform_module"},
				schemaDescription: "Filter by package type: composer, conan, generic, golang, helm, maven, npm, nuget, pypi, or terraform_module.",
			}),
		}
	}
	if actionName == "publish" {
		options.InputSchemaOverrides = []toolutil.InputSchemaOverride{
			toolutil.SchemaPropertyOverride("status", map[string]any{
				schemaEnum:        []any{"default", "hidden"},
				schemaDescription: "Package visibility on publish: default (publicly visible) or hidden.",
			}),
			toolutil.SchemaPropertyOverride("select", map[string]any{
				schemaEnum:        []any{"package_file"},
				schemaDescription: "Response detail selector. Use package_file to receive the published file metadata (id, size, checksums).",
			}),
		}
	}
	if actionName == actionNamePublishDir {
		options.Usage = "Publish all regular files from a local directory to Generic Packages. Omit include_pattern to upload every file. include_pattern is one glob, not a comma-separated file list."
		options.Aliases = []string{"publish local directory", "upload package directory", "generic package directory upload", "publish multiple package files", "publish files from directory", "upload package files from directory", "generic package publish directory files", "publish fixture files directory"}
		options.Tags = append(options.Tags, "generic_package", "directory_upload", "package-files-directory", "fixture-files-directory")
		options.RelatedActions = []string{"release.create", "release.link_create_batch", actionPackagePublish}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"include_pattern": {
				SemanticRole: "single_glob_filter",
				ValueSource:  "Optional single glob matched against file names inside directory_path. Omit it to include all regular files.",
				CommonConfusions: []string{
					"Do not pass comma-separated filenames.",
					"Do not use include_pattern to enumerate exact files. Omit it when all fixture files should be uploaded.",
				},
			},
		}
	}
	return options
}
