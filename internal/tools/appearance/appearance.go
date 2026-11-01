// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package appearance

import (
	"context"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Item represents the application appearance in output.
type Item struct {
	SiteName                    string `json:"site_name,omitempty"`
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
		SiteName:                    a.SiteName,
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
		return GetOutput{}, toolutil.WrapErrWithStatusHint("appearance_get", err, http.StatusForbidden, "appearance settings require administrator access")
	}
	return GetOutput{Appearance: toItem(a)}, nil
}

// Update.

// UpdateInput is the input for changing appearance.
//
// The texts and the two colors are pointers so the empty string that clears
// one reaches GitLab: lib/api/appearance.rb declares each an optional String
// and app/models/appearance.rb neither requires one nor refuses a blank color
// (issue 1306). The four image fields stay strings, since each is part of an
// upload rather than a stored value. The route declares no url, so there is
// none here: client-go's ChangeAppearanceOptions.URL sends a key GitLab
// ignores.
type UpdateInput struct {
	SiteName                    *string `json:"site_name,omitempty" jsonschema:"Site name GitLab appends after the page title in the browser tab. Pass an empty string to remove it. Omit the field to leave it untouched"`
	Title                       *string `json:"title,omitempty" jsonschema:"Application title displayed in the header. Pass an empty string to remove it. Omit the field to leave it untouched"`
	Description                 *string `json:"description,omitempty" jsonschema:"Instance description on sign-in page. Pass an empty string to remove it. Omit the field to leave it untouched"`
	PWAName                     *string `json:"pwa_name,omitempty" jsonschema:"Progressive Web App name. Pass an empty string to remove it. Omit the field to leave it untouched"`
	PWAShortName                *string `json:"pwa_short_name,omitempty" jsonschema:"PWA short name. Pass an empty string to remove it. Omit the field to leave it untouched"`
	PWADescription              *string `json:"pwa_description,omitempty" jsonschema:"PWA description. Pass an empty string to remove it. Omit the field to leave it untouched"`
	PWAIcon                     string  `json:"pwa_icon,omitempty" jsonschema:"Path to the PWA icon image asset"`
	Logo                        string  `json:"logo,omitempty" jsonschema:"Path to the instance logo image asset"`
	HeaderLogo                  string  `json:"header_logo,omitempty" jsonschema:"Path to the header logo image asset"`
	Favicon                     string  `json:"favicon,omitempty" jsonschema:"Path to the favicon image asset"`
	HeaderMessage               *string `json:"header_message,omitempty" jsonschema:"Message in header bar. Pass an empty string to remove it. Omit the field to leave it untouched"`
	FooterMessage               *string `json:"footer_message,omitempty" jsonschema:"Message in footer bar. Pass an empty string to remove it. Omit the field to leave it untouched"`
	MessageBackgroundColor      *string `json:"message_background_color,omitempty" jsonschema:"Background color for header/footer messages (hex). Pass an empty string to remove it. Omit the field to leave it untouched"`
	MessageFontColor            *string `json:"message_font_color,omitempty" jsonschema:"Font color for header/footer messages (hex). Pass an empty string to remove it. Omit the field to leave it untouched"`
	EmailHeaderAndFooterEnabled *bool   `json:"email_header_and_footer_enabled,omitempty" jsonschema:"Enable header and footer in emails"`
	MemberGuidelines            *string `json:"member_guidelines,omitempty" jsonschema:"Markdown guidelines for adding members. Pass an empty string to remove them. Omit the field to leave them untouched"`
	NewProjectGuidelines        *string `json:"new_project_guidelines,omitempty" jsonschema:"Markdown guidelines for new projects. Pass an empty string to remove them. Omit the field to leave them untouched"`
	ProfileImageGuidelines      *string `json:"profile_image_guidelines,omitempty" jsonschema:"Markdown guidelines for profile images. Pass an empty string to remove them. Omit the field to leave them untouched"`
}

// UpdateOutput contains the updated appearance.
type UpdateOutput struct {
	toolutil.HintableOutput
	Appearance Item `json:"appearance"`
}

// changeAppearanceOptions copies the fields the caller set into the SDK's
// options struct, leaving the rest nil so GitLab keeps their current values.
// A text or color is copied as the pointer it arrived as, so nil decides that
// it was left out and "" reaches GitLab as the value that clears it.
//
// It is a function of its own because the copy is one line per field, and
// with them inline the request and its two failure modes sat at the bottom of
// eighteen of them.
func changeAppearanceOptions(input UpdateInput) *gl.ChangeAppearanceOptions {
	opts := &gl.ChangeAppearanceOptions{
		SiteName:               input.SiteName,
		Title:                  input.Title,
		Description:            input.Description,
		PWAName:                input.PWAName,
		PWAShortName:           input.PWAShortName,
		PWADescription:         input.PWADescription,
		HeaderMessage:          input.HeaderMessage,
		FooterMessage:          input.FooterMessage,
		MessageBackgroundColor: input.MessageBackgroundColor,
		MessageFontColor:       input.MessageFontColor,
		MemberGuidelines:       input.MemberGuidelines,
		NewProjectGuidelines:   input.NewProjectGuidelines,
		ProfileImageGuidelines: input.ProfileImageGuidelines,
	}
	if input.PWAIcon != "" {
		opts.PWAIcon = new(input.PWAIcon)
	}
	if input.Logo != "" {
		opts.Logo = new(input.Logo)
	}
	if input.HeaderLogo != "" {
		opts.HeaderLogo = new(input.HeaderLogo)
	}
	if input.Favicon != "" {
		opts.Favicon = new(input.Favicon)
	}
	if input.EmailHeaderAndFooterEnabled != nil {
		opts.EmailHeaderAndFooterEnabled = input.EmailHeaderAndFooterEnabled
	}
	return opts
}

// Update changes the application appearance (admin-only).
func Update(ctx context.Context, client *gitlabclient.Client, input UpdateInput) (UpdateOutput, error) {
	opts := changeAppearanceOptions(input)

	a, _, err := client.GL().Appearance.ChangeAppearance(opts, gl.WithContext(ctx))
	if err != nil {
		return UpdateOutput{}, toolutil.WrapErrWithStatusHint("appearance_update", err, http.StatusForbidden, "updating appearance requires administrator access")
	}
	return UpdateOutput{Appearance: toItem(a)}, nil
}
