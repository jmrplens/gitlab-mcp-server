package alertmanagement

import (
	"context"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ListMetricImages.

// ListMetricImagesInput contains parameters for listing alert metric images.
type ListMetricImagesInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	AlertIID  int64                `json:"alert_iid" jsonschema:"Alert IID,required"`
	// OrderBy names the column used to order keyset-paginated results.
	OrderBy string `json:"order_by,omitempty" jsonschema:"Column to order keyset-paginated results by (e.g. created_at, id)"`
	// Sort selects the sort direction for ordered results.
	Sort string `json:"sort,omitempty" jsonschema:"Sort direction (asc or desc)"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// MetricImageItem represents a single metric image.
type MetricImageItem struct {
	toolutil.HintableOutput
	ID        int64  `json:"id"`
	CreatedAt string `json:"created_at,omitempty"`
	Filename  string `json:"filename"`
	FilePath  string `json:"file_path"`
	URL       string `json:"url"`
	URLText   string `json:"url_text"`
}

// ListMetricImagesOutput contains a list of metric images.
type ListMetricImagesOutput struct {
	toolutil.HintableOutput
	Images     []MetricImageItem         `json:"images"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// newMetricImageItem maps a client-go MetricImage onto the MCP output item,
// mirroring every SDK field (id, created_at, filename, file_path, url, url_text).
func newMetricImageItem(img *gl.MetricImage) MetricImageItem {
	item := MetricImageItem{
		ID:       img.ID,
		Filename: img.Filename,
		FilePath: img.FilePath,
		URL:      img.URL,
		URLText:  img.URLText,
	}
	if img.CreatedAt != nil {
		item.CreatedAt = img.CreatedAt.Format(time.RFC3339)
	}
	return item
}

// ListMetricImages retrieves metric images for an alert.
func ListMetricImages(ctx context.Context, client *gitlabclient.Client, input ListMetricImagesInput) (ListMetricImagesOutput, error) {
	if input.AlertIID <= 0 {
		return ListMetricImagesOutput{}, toolutil.ErrRequiredInt64("gitlab_list_alert_metric_images", "alert_iid")
	}
	opts := &gl.ListMetricImagesOptions{
		OrderBy: input.OrderBy, Sort: input.Sort,
	}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	images, resp, err := client.GL().AlertManagement.ListMetricImages(string(input.ProjectID), input.AlertIID, opts, gl.WithContext(ctx))
	if err != nil {
		return ListMetricImagesOutput{}, toolutil.WrapErrWithStatusHint("gitlab_list_alert_metric_images", err, http.StatusNotFound, "verify project_id and alert_iid \u2014 check alerts with the project's alert management")
	}
	items := make([]MetricImageItem, 0, len(images))
	for _, img := range images {
		items = append(items, newMetricImageItem(img))
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
		return MetricImageItem{}, toolutil.WrapErrWithStatusHint("gitlab_update_alert_metric_image", err, http.StatusNotFound, "verify image_id with gitlab_list_alert_metric_images")
	}
	return newMetricImageItem(img), nil
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

	reader, err := toolutil.ReadFileOrBase64("gitlab_upload_alert_metric_image", input.FilePathLocal, input.ContentBase64)
	if err != nil {
		return MetricImageItem{}, err
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
		return MetricImageItem{}, toolutil.WrapErrWithStatusHint("gitlab_upload_alert_metric_image", err, http.StatusBadRequest, "check file content is valid base64 PNG/JPEG")
	}
	return newMetricImageItem(img), nil
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
		return toolutil.WrapErrWithStatusHint("gitlab_delete_alert_metric_image", err, http.StatusNotFound, "verify image_id with gitlab_list_alert_metric_images")
	}
	return nil
}

// formatters.
