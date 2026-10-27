// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package toolutil

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"

	retryablehttp "github.com/hashicorp/go-retryablehttp"
	gl "gitlab.com/gitlab-org/api/client-go/v3"
)

// BodyEdit says what [EditJSONBody] writes into the JSON object client-go
// encoded as a request body, and what it removes from it.
type BodyEdit struct {
	// Set writes each key with its value, replacing whatever client-go put
	// there. A nil value writes null, which is how GitLab is told to remove a
	// value an option struct tagged omitempty cannot send.
	Set map[string]any
	// Drop removes each key, for a field an option struct writes on every
	// call although the caller left it out.
	Drop []string
}

// NullWhenZero reads one optional update field that GitLab clears only with
// null, for an option struct whose pointer is tagged omitempty and so cannot
// carry one. A field left out (nil) returns nil and edits nothing. A field
// given its zero value, "" for a text or 0 for an ID reference, which is how a
// caller asks to clear it, records key as null in edit and returns nil, so the
// option struct leaves the key to the edit. Any other value is returned as
// given, for the option struct to send.
func NullWhenZero[T comparable](value *T, key string, edit *BodyEdit) *T {
	var zero T
	if value == nil || *value != zero {
		return value
	}
	if edit.Set == nil {
		edit.Set = map[string]any{}
	}
	edit.Set[key] = nil
	return nil
}

// EditJSONBody returns the request option that applies edit to the body of a
// write client-go has encoded, for the values an update has to send that its
// option struct cannot express: a null that clears a field whose pointer is
// tagged omitempty, an empty list a slice tagged omitempty leaves out, and the
// removal of a key a field without omitempty writes as null on every call. An
// edit with nothing in it returns no option, so the request goes out exactly as
// client-go built it.
//
// It is meant for the POST, PUT and PATCH routes, whose options client-go
// writes as a JSON object. A body that is not one, or cannot be read, fails the
// request rather than sending it without the edit it was asked to carry.
func EditJSONBody(edit BodyEdit) gl.RequestOptionFunc {
	if len(edit.Set) == 0 && len(edit.Drop) == 0 {
		return nil
	}
	return func(req *retryablehttp.Request) error {
		raw, err := req.BodyBytes()
		if err != nil {
			return fmt.Errorf("read the request body to edit it: %w", err)
		}
		body := map[string]any{}
		if len(raw) > 0 {
			decoder := json.NewDecoder(bytes.NewReader(raw))
			// Numbers stay as written, so an ID beside the edit is sent as
			// client-go encoded it rather than through a float64.
			decoder.UseNumber()
			if err = decoder.Decode(&body); err != nil {
				return fmt.Errorf("decode the request body to edit it: %w", err)
			}
		}
		for _, key := range edit.Drop {
			delete(body, key)
		}
		maps.Copy(body, edit.Set)
		// A map of what encoding/json decoded, plus values the handler chose,
		// marshals without error, and SetBody takes a byte slice without one;
		// both are joined rather than branched on so that neither impossible
		// failure is left untested.
		rewritten, marshalErr := json.Marshal(body)
		return errors.Join(marshalErr, req.SetBody(rewritten))
	}
}
