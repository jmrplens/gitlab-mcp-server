// Package dependencies implements MCP tool handlers for GitLab dependency
// listing and dependency list export (SBOM) operations. It wraps the
// Dependencies and DependencyListExport services from client-go v2.
package dependencies

import (
	"context"
	"io"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ListInput defines parameters for listing project dependencies.
type ListInput struct {
	ProjectID      toolutil.StringOrInt `json:"project_id"                jsonschema:"Project ID or URL-encoded path,required"`
	PackageManager string               `json:"package_manager,omitempty" jsonschema:"Filter by package manager (bundler, composer, conan, go, gradle, maven, npm, nuget, pip, pipenv, pnpm, yarn, sbt, setuptools)"`
	toolutil.PaginationInput
}

// CreateExportInput defines parameters for creating a dependency list export.
type CreateExportInput struct {
	PipelineID int64  `json:"pipeline_id" jsonschema:"Pipeline ID to export dependencies from,required"`
	ExportType string `json:"export_type,omitempty" jsonschema:"Export type (default: sbom)"`
}

// GetExportInput defines parameters for checking a dependency list export status.
type GetExportInput struct {
	ExportID int64 `json:"export_id" jsonschema:"Dependency list export ID,required"`
}

// DownloadExportInput defines parameters for downloading a dependency list export.
type DownloadExportInput struct {
	ExportID int64 `json:"export_id" jsonschema:"Dependency list export ID,required"`
}

// VulnerabilityOutput represents a vulnerability found in a dependency.
type VulnerabilityOutput struct {
	Name     string `json:"name"`
	Severity string `json:"severity"`
	ID       int64  `json:"id"`
	URL      string `json:"url,omitempty"`
}

// LicenseOutput represents a license of a dependency.
type LicenseOutput struct {
	Name string `json:"name"`
	URL  string `json:"url,omitempty"`
}

// Output represents a single project dependency.
type Output struct {
	Name               string                `json:"name"`
	Version            string                `json:"version"`
	PackageManager     string                `json:"package_manager"`
	DependencyFilePath string                `json:"dependency_file_path"`
	Vulnerabilities    []VulnerabilityOutput `json:"vulnerabilities,omitempty"`
	Licenses           []LicenseOutput       `json:"licenses,omitempty"`
}

// ListOutput holds a paginated list of dependencies.
type ListOutput struct {
	toolutil.HintableOutput
	Dependencies []Output                  `json:"dependencies"`
	Pagination   toolutil.PaginationOutput `json:"pagination"`
}

// ExportOutput represents a dependency list export.
type ExportOutput struct {
	toolutil.HintableOutput
	ID          int64  `json:"id"`
	HasFinished bool   `json:"has_finished"`
	Self        string `json:"self"`
	Download    string `json:"download"`
}

// DownloadOutput holds the raw content of a downloaded SBOM export.
type DownloadOutput struct {
	toolutil.HintableOutput
	Content string `json:"content"`
}

func toOutput(d *gl.Dependency) Output {
	o := Output{
		Name:               d.Name,
		Version:            d.Version,
		PackageManager:     string(d.PackageManager),
		DependencyFilePath: d.DependencyFilePath,
	}
	for _, v := range d.Vulnerabilities {
		o.Vulnerabilities = append(o.Vulnerabilities, VulnerabilityOutput{
			Name:     v.Name,
			Severity: v.Severity,
			ID:       v.ID,
			URL:      v.URL,
		})
	}
	for _, l := range d.Licenses {
		o.Licenses = append(o.Licenses, LicenseOutput{
			Name: l.Name,
			URL:  l.URL,
		})
	}
	return o
}

func toExportOutput(e *gl.DependencyListExport) ExportOutput {
	return ExportOutput{
		ID:          e.ID,
		HasFinished: e.HasFinished,
		Self:        e.Self,
		Download:    e.Download,
	}
}

// ListDeps lists dependencies for a project.
func ListDeps(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	if input.ProjectID == "" {
		return ListOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	opts := &gl.ListProjectDependenciesOptions{}
	if input.PackageManager != "" {
		pm := gl.DependencyPackageManagerValue(input.PackageManager)
		opts.PackageManager = []*gl.DependencyPackageManagerValue{&pm}
	}
	if input.Page > 0 {
		opts.Page = int64(input.Page)
	}
	if input.PerPage > 0 {
		opts.PerPage = int64(input.PerPage)
	}
	deps, resp, err := client.GL().Dependencies.ListProjectDependencies(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("dependencyList", err)
	}
	out := make([]Output, len(deps))
	for i, d := range deps {
		out[i] = toOutput(d)
	}
	return ListOutput{Dependencies: out, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// CreateExport creates a dependency list export for a pipeline.
func CreateExport(ctx context.Context, client *gitlabclient.Client, input CreateExportInput) (ExportOutput, error) {
	if err := ctx.Err(); err != nil {
		return ExportOutput{}, err
	}
	if input.PipelineID <= 0 {
		return ExportOutput{}, toolutil.ErrRequiredInt64("dependencyCreateExport", "pipeline_id")
	}
	opts := &gl.CreateDependencyListExportOptions{}
	if input.ExportType != "" {
		opts.ExportType = new(input.ExportType)
	}
	e, _, err := client.GL().DependencyListExport.CreateDependencyListExport(input.PipelineID, opts, gl.WithContext(ctx))
	if err != nil {
		return ExportOutput{}, toolutil.WrapErrWithMessage("dependencyCreateExport", err)
	}
	return toExportOutput(e), nil
}

// GetExport checks the status of a dependency list export.
func GetExport(ctx context.Context, client *gitlabclient.Client, input GetExportInput) (ExportOutput, error) {
	if err := ctx.Err(); err != nil {
		return ExportOutput{}, err
	}
	if input.ExportID <= 0 {
		return ExportOutput{}, toolutil.ErrRequiredInt64("dependencyGetExport", "export_id")
	}
	e, _, err := client.GL().DependencyListExport.GetDependencyListExport(input.ExportID, gl.WithContext(ctx))
	if err != nil {
		return ExportOutput{}, toolutil.WrapErrWithMessage("dependencyGetExport", err)
	}
	return toExportOutput(e), nil
}

// DownloadExport downloads the content of a dependency list export (SBOM).
func DownloadExport(ctx context.Context, client *gitlabclient.Client, input DownloadExportInput) (DownloadOutput, error) {
	if err := ctx.Err(); err != nil {
		return DownloadOutput{}, err
	}
	if input.ExportID <= 0 {
		return DownloadOutput{}, toolutil.ErrRequiredInt64("dependencyDownloadExport", "export_id")
	}
	rc, _, err := client.GL().DependencyListExport.DownloadDependencyListExport(input.ExportID, gl.WithContext(ctx))
	if err != nil {
		return DownloadOutput{}, toolutil.WrapErrWithMessage("dependencyDownloadExport", err)
	}
	defer rc.Close()

	// Limit read to 1MB to prevent excessive memory usage
	const maxSize = 1 << 20
	limited := io.LimitReader(rc, maxSize)
	data, err := io.ReadAll(limited)
	if err != nil {
		return DownloadOutput{}, toolutil.WrapErrWithMessage("dependencyDownloadExport", err)
	}
	return DownloadOutput{Content: string(data)}, nil
}
