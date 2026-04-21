// Package appearance implements MCP tool handlers for GitLab application appearance.
// It wraps the AppearanceService from client-go v2.
// These are admin-only endpoints requiring administrator access.
package appearance

import (
	"context"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// Item represents the application appearance in output.
type Item struct {
	Title                       string `json:"title"`
	Description                 string `json:"description,omitempty"`
	PWAName                     string `json:"pwa_name,omitempty"`
	PWAShortName                string `json:"pwa_short_name,omitempty"`
	PWADescription              string `json:"pwa_description,omitempty"`
	PWAIcon                     string `json:"pwa_icon,omitempty"`
	Logo                        string `json:"logo,omitempty"`
	HeaderLogo                  string `json:"header_logo,omitempty"`
	Favicon                     string `json:"favicon,omitempty"`
	MemberGuidelines            string `json:"member_guidelines,omitempty"`
	NewProjectGuidelines        string `json:"new_project_guidelines,omitempty"`
	ProfileImageGuidelines      string `json:"profile_image_guidelines,omitempty"`
	HeaderMessage               string `json:"header_message,omitempty"`
	FooterMessage               string `json:"footer_message,omitempty"`
	MessageBackgroundColor      string `json:"message_background_color,omitempty"`
	MessageFontColor            string `json:"message_font_color,omitempty"`
	EmailHeaderAndFooterEnabled bool   `json:"email_header_and_footer_enabled"`
}

// toItem converts the GitLab API response to the tool output format.
func toItem(a *gl.Appearance) Item {
	return Item{
		Title:                       a.Title,
		Description:                 a.Description,
		PWAName:                     a.PWAName,
		PWAShortName:                a.PWAShortName,
		PWADescription:              a.PWADescription,
		PWAIcon:                     a.PWAIcon,
		Logo:                        a.Logo,
		HeaderLogo:                  a.HeaderLogo,
		Favicon:                     a.Favicon,
		MemberGuidelines:            a.MemberGuidelines,
		NewProjectGuidelines:        a.NewProjectGuidelines,
		ProfileImageGuidelines:      a.ProfileImageGuidelines,
		HeaderMessage:               a.HeaderMessage,
		FooterMessage:               a.FooterMessage,
		MessageBackgroundColor:      a.MessageBackgroundColor,
		MessageFontColor:            a.MessageFontColor,
		EmailHeaderAndFooterEnabled: a.EmailHeaderAndFooterEnabled,
	}
}

// Get.

// GetInput is the input for getting appearance (no parameters needed).
type GetInput struct{}

// GetOutput contains the application appearance.
type GetOutput struct {
	toolutil.HintableOutput
	Appearance Item `json:"appearance"`
}

// Get retrieves the current application appearance (admin-only).
func Get(ctx context.Context, client *gitlabclient.Client, _ GetInput) (GetOutput, error) {
	a, _, err := client.GL().Appearance.GetAppearance(gl.WithContext(ctx))
	if err != nil {
		return GetOutput{}, toolutil.WrapErrWithMessage("appearance_get", err)
	}
	return GetOutput{Appearance: toItem(a)}, nil
}

// Update.

// UpdateInput is the input for changing appearance.
type UpdateInput struct {
	Title                       string `json:"title,omitempty" jsonschema:"Application title displayed in the header"`
	Description                 string `json:"description,omitempty" jsonschema:"Instance description on sign-in page"`
	PWAName                     string `json:"pwa_name,omitempty" jsonschema:"Progressive Web App name"`
	PWAShortName                string `json:"pwa_short_name,omitempty" jsonschema:"PWA short name"`
	PWADescription              string `json:"pwa_description,omitempty" jsonschema:"PWA description"`
	HeaderMessage               string `json:"header_message,omitempty" jsonschema:"Message in header bar"`
	FooterMessage               string `json:"footer_message,omitempty" jsonschema:"Message in footer bar"`
	MessageBackgroundColor      string `json:"message_background_color,omitempty" jsonschema:"Background color for header/footer messages (hex)"`
	MessageFontColor            string `json:"message_font_color,omitempty" jsonschema:"Font color for header/footer messages (hex)"`
	EmailHeaderAndFooterEnabled *bool  `json:"email_header_and_footer_enabled,omitempty" jsonschema:"Enable header and footer in emails"`
	MemberGuidelines            string `json:"member_guidelines,omitempty" jsonschema:"Markdown guidelines for adding members"`
	NewProjectGuidelines        string `json:"new_project_guidelines,omitempty" jsonschema:"Markdown guidelines for new projects"`
	ProfileImageGuidelines      string `json:"profile_image_guidelines,omitempty" jsonschema:"Markdown guidelines for profile images"`
}

// UpdateOutput contains the updated appearance.
type UpdateOutput struct {
	toolutil.HintableOutput
	Appearance Item `json:"appearance"`
}

// Update changes the application appearance (admin-only).
func Update(ctx context.Context, client *gitlabclient.Client, input UpdateInput) (UpdateOutput, error) {
	opts := &gl.ChangeAppearanceOptions{}
	if input.Title != "" {
		opts.Title = new(input.Title)
	}
	if input.Description != "" {
		opts.Description = new(input.Description)
	}
	if input.PWAName != "" {
		opts.PWAName = new(input.PWAName)
	}
	if input.PWAShortName != "" {
		opts.PWAShortName = new(input.PWAShortName)
	}
	if input.PWADescription != "" {
		opts.PWADescription = new(input.PWADescription)
	}
	if input.HeaderMessage != "" {
		opts.HeaderMessage = new(input.HeaderMessage)
	}
	if input.FooterMessage != "" {
		opts.FooterMessage = new(input.FooterMessage)
	}
	if input.MessageBackgroundColor != "" {
		opts.MessageBackgroundColor = new(input.MessageBackgroundColor)
	}
	if input.MessageFontColor != "" {
		opts.MessageFontColor = new(input.MessageFontColor)
	}
	if input.EmailHeaderAndFooterEnabled != nil {
		opts.EmailHeaderAndFooterEnabled = input.EmailHeaderAndFooterEnabled
	}
	if input.MemberGuidelines != "" {
		opts.MemberGuidelines = new(input.MemberGuidelines)
	}
	if input.NewProjectGuidelines != "" {
		opts.NewProjectGuidelines = new(input.NewProjectGuidelines)
	}
	if input.ProfileImageGuidelines != "" {
		opts.ProfileImageGuidelines = new(input.ProfileImageGuidelines)
	}

	a, _, err := client.GL().Appearance.ChangeAppearance(opts, gl.WithContext(ctx))
	if err != nil {
		return UpdateOutput{}, toolutil.WrapErrWithMessage("appearance_update", err)
	}
	return UpdateOutput{Appearance: toItem(a)}, nil
}
