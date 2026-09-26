package awardemoji

import (
	"fmt"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// awardExtra is the one key of lib/api/entities/award_emoji.rb that
// client-go's AwardEmoji does not model, read from the captured response
// beside the SDK's own decode (ADR-0021). The gap is recorded in
// docs/development/upstream-bugs.md.
//
// The entity exposes url with no condition, and app/models/award_emoji.rb
// fills it only for a custom emoji, the group's own image: a standard emoji
// sends null, which leaves the field empty and absent.
type awardExtra struct {
	URL string `json:"url"`
}

// capturedOutput converts one award and adds the image URL read off the
// captured answer to the request that returned it.
func capturedOutput(operation string, emoji *gl.AwardEmoji, capture *gitlabclient.ResponseCapture) (Output, error) {
	var extra awardExtra
	if err := capture.Decode(&extra); err != nil {
		return Output{}, toolutil.WrapErr(operation, err)
	}
	out := toOutput(emoji)
	out.URL = extra.URL
	return out, nil
}

// capturedAwards reads the extras off a list answer, one per award in order,
// the count held to what the SDK decoded so no URL lands on another award.
func capturedAwards(capture *gitlabclient.ResponseCapture, decoded int) ([]awardExtra, error) {
	var extras []awardExtra
	if err := capture.Decode(&extras); err != nil {
		return nil, err
	}
	if len(extras) != decoded {
		return nil, fmt.Errorf("the captured answer holds %d awards and the SDK decoded %d", len(extras), decoded)
	}
	return extras, nil
}

// capturedListOutput converts a page of awards and adds each one's image URL.
func capturedListOutput(operation string, emojis []*gl.AwardEmoji, resp *gl.Response, capture *gitlabclient.ResponseCapture) (ListOutput, error) {
	extras, err := capturedAwards(capture, len(emojis))
	if err != nil {
		return ListOutput{}, toolutil.WrapErr(operation, err)
	}
	out := toListOutput(emojis, resp)
	for i := range out.AwardEmoji {
		out.AwardEmoji[i].URL = extras[i].URL
	}
	return out, nil
}
