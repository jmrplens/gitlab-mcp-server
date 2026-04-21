// Package alertmanagement implements MCP tools for GitLab Alert Management metric images.
package alertmanagement

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ListMetricImages.

// ListMetricImagesInput contains parameters for listing alert metric images.
type ListMetricImagesInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	AlertIID  int64                `json:"alert_iid" jsonschema:"Alert IID,required"`
	Page      int64                `json:"page" jsonschema:"Page number for pagination"`
	PerPage   int64                `json:"per_page" jsonschema:"Number of items per page"`
}

// MetricImageItem represents a single metric image.
type MetricImageItem struct {
	toolutil.HintableOutput
	ID       int64  `json:"id"`
	Filename string `json:"filename"`
	FilePath string `json:"file_path"`
	URL      string `json:"url"`
	URLText  string `json:"url_text"`
}

// ListMetricImagesOutput contains a list of metric images.
type ListMetricImagesOutput struct {
	toolutil.HintableOutput
	Images     []MetricImageItem         `json:"images"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// ListMetricImages retrieves metric images for an alert.
func ListMetricImages(ctx context.Context, client *gitlabclient.Client, input ListMetricImagesInput) (ListMetricImagesOutput, error) {
	if input.AlertIID <= 0 {
		return ListMetricImagesOutput{}, toolutil.ErrRequiredInt64("gitlab_list_alert_metric_images", "alert_iid")
	}
	opts := &gl.ListMetricImagesOptions{}
	if input.Page > 0 || input.PerPage > 0 {
		opts.ListOptions = gl.ListOptions{Page: input.Page, PerPage: input.PerPage}
	}
	images, resp, err := client.GL().AlertManagement.ListMetricImages(string(input.ProjectID), input.AlertIID, opts, gl.WithContext(ctx))
	if err != nil {
		return ListMetricImagesOutput{}, toolutil.WrapErrWithMessage("gitlab_list_alert_metric_images", err)
	}
	items := make([]MetricImageItem, 0, len(images))
	for _, img := range images {
		items = append(items, MetricImageItem{
			ID:       img.ID,
			Filename: img.Filename,
			FilePath: img.FilePath,
			URL:      img.URL,
			URLText:  img.URLText,
		})
	}
	return ListMetricImagesOutput{
		Images:     items,
		Pagination: toolutil.PaginationFromResponse(resp),
	}, nil
}

// UpdateMetricImage.

// UpdateMetricImageInput contains parameters for updating a metric image.
type UpdateMetricImageInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	AlertIID  int64                `json:"alert_iid" jsonschema:"Alert IID,required"`
	ImageID   int64                `json:"image_id" jsonschema:"Metric image ID,required"`
	URL       *string              `json:"url" jsonschema:"URL to link the metric image to"`
	URLText   *string              `json:"url_text" jsonschema:"Text for the URL link"`
}

// UpdateMetricImage updates a metric image for an alert.
func UpdateMetricImage(ctx context.Context, client *gitlabclient.Client, input UpdateMetricImageInput) (MetricImageItem, error) {
	if input.AlertIID <= 0 {
		return MetricImageItem{}, toolutil.ErrRequiredInt64("gitlab_update_alert_metric_image", "alert_iid")
	}
	if input.ImageID <= 0 {
		return MetricImageItem{}, toolutil.ErrRequiredInt64("gitlab_update_alert_metric_image", "image_id")
	}
	opts := &gl.UpdateMetricImageOptions{
		URL:     input.URL,
		URLText: input.URLText,
	}
	img, _, err := client.GL().AlertManagement.UpdateMetricImage(string(input.ProjectID), input.AlertIID, input.ImageID, opts, gl.WithContext(ctx))
	if err != nil {
		return MetricImageItem{}, toolutil.WrapErrWithMessage("gitlab_update_alert_metric_image", err)
	}
	return MetricImageItem{
		ID:       img.ID,
		Filename: img.Filename,
		FilePath: img.FilePath,
		URL:      img.URL,
		URLText:  img.URLText,
	}, nil
}

// UploadMetricImage.

// UploadMetricImageInput contains parameters for uploading a metric image.
// Exactly one of FilePath or ContentBase64 must be provided.
type UploadMetricImageInput struct {
	ProjectID     toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	AlertIID      int64                `json:"alert_iid" jsonschema:"Alert IID,required"`
	FilePathLocal string               `json:"file_path,omitempty" jsonschema:"Absolute path to a local image file on the MCP server filesystem. Alternative to content_base64. Only one of file_path or content_base64 should be provided."`
	ContentBase64 string               `json:"content_base64,omitempty" jsonschema:"Base64-encoded image content. Only one of file_path or content_base64 should be provided."`
	Filename      string               `json:"filename" jsonschema:"Image filename,required"`
	URL           *string              `json:"url" jsonschema:"URL to link the metric image to"`
	URLText       *string              `json:"url_text" jsonschema:"Text for the URL link"`
}

// UploadMetricImage uploads a metric image for an alert.
func UploadMetricImage(ctx context.Context, client *gitlabclient.Client, input UploadMetricImageInput) (MetricImageItem, error) {
	if input.AlertIID <= 0 {
		return MetricImageItem{}, toolutil.ErrRequiredInt64("gitlab_upload_alert_metric_image", "alert_iid")
	}

	hasFilePath := input.FilePathLocal != ""
	hasBase64 := input.ContentBase64 != ""

	if hasFilePath && hasBase64 {
		return MetricImageItem{}, errors.New("gitlab_upload_alert_metric_image: provide either file_path or content_base64, not both")
	}
	if !hasFilePath && !hasBase64 {
		return MetricImageItem{}, errors.New("gitlab_upload_alert_metric_image: either file_path or content_base64 is required")
	}

	var reader *bytes.Reader

	if hasFilePath {
		cfg := toolutil.GetUploadConfig()
		f, info, err := toolutil.OpenAndValidateFile(input.FilePathLocal, cfg.MaxFileSize)
		if err != nil {
			return MetricImageItem{}, fmt.Errorf("gitlab_upload_alert_metric_image: %w", err)
		}
		defer f.Close()

		data := make([]byte, info.Size())
		if _, err = io.ReadFull(f, data); err != nil {
			return MetricImageItem{}, fmt.Errorf("gitlab_upload_alert_metric_image: reading file: %w", err)
		}
		reader = bytes.NewReader(data)
	} else {
		decoded, err := base64.StdEncoding.DecodeString(input.ContentBase64)
		if err != nil {
			return MetricImageItem{}, fmt.Errorf("gitlab_upload_alert_metric_image: invalid base64 content: %w", err)
		}
		reader = bytes.NewReader(decoded)
	}

	uploadOpts := &gl.UploadMetricImageOptions{}
	if input.URL != nil {
		uploadOpts.URL = input.URL
	}
	if input.URLText != nil {
		uploadOpts.URLText = input.URLText
	}
	img, _, err := client.GL().AlertManagement.UploadMetricImage(string(input.ProjectID), input.AlertIID, reader, input.Filename, uploadOpts, gl.WithContext(ctx))
	if err != nil {
		return MetricImageItem{}, toolutil.WrapErrWithMessage("gitlab_upload_alert_metric_image", err)
	}
	return MetricImageItem{
		ID:       img.ID,
		Filename: img.Filename,
		FilePath: img.FilePath,
		URL:      img.URL,
		URLText:  img.URLText,
	}, nil
}

// DeleteMetricImage.

// DeleteMetricImageInput contains parameters for deleting a metric image.
type DeleteMetricImageInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	AlertIID  int64                `json:"alert_iid" jsonschema:"Alert IID,required"`
	ImageID   int64                `json:"image_id" jsonschema:"Metric image ID,required"`
}

// DeleteMetricImage deletes a metric image for an alert.
func DeleteMetricImage(ctx context.Context, client *gitlabclient.Client, input DeleteMetricImageInput) error {
	if input.AlertIID <= 0 {
		return toolutil.ErrRequiredInt64("gitlab_delete_alert_metric_image", "alert_iid")
	}
	if input.ImageID <= 0 {
		return toolutil.ErrRequiredInt64("gitlab_delete_alert_metric_image", "image_id")
	}
	_, err := client.GL().AlertManagement.DeleteMetricImage(string(input.ProjectID), input.AlertIID, input.ImageID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithMessage("gitlab_delete_alert_metric_image", err)
	}
	return nil
}

// formatters.
