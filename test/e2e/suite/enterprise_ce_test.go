//go:build e2e && !enterprise

// enterprise_ce_test.go verifies CE catalog behavior for Enterprise-only tools.
package suite

import (
	"context"
	"testing"
	"time"
)

// TestEnterpriseSecurityTools_NotRegisteredOnCE verifies that CE E2E runs do
// not expose Premium/Ultimate security classification tools at all.
func TestEnterpriseSecurityTools_NotRegisteredOnCE(t *testing.T) {
	t.Parallel()
	if sess.enterprise {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	individual, err := sess.individual.ListTools(ctx, nil)
	requireNoError(t, err, "list individual tools")
	individualForbidden := map[string]bool{
		"gitlab_bulk_update_security_attributes":   true,
		"gitlab_create_security_attribute":         true,
		"gitlab_create_security_category":          true,
		"gitlab_delete_security_attribute":         true,
		"gitlab_delete_security_category":          true,
		"gitlab_project_update_security_attribute": true,
		"gitlab_update_security_attribute":         true,
		"gitlab_update_security_category":          true,
	}
	for _, tool := range individual.Tools {
		if individualForbidden[tool.Name] {
			t.Fatalf("CE individual surface exposed enterprise tool %q", tool.Name)
		}
	}

	meta, err := sess.meta.ListTools(ctx, nil)
	requireNoError(t, err, "list meta-tools")
	metaForbidden := map[string]bool{
		"gitlab_security_attribute": true,
		"gitlab_security_category":  true,
	}
	for _, tool := range meta.Tools {
		if metaForbidden[tool.Name] {
			t.Fatalf("CE meta surface exposed enterprise tool %q", tool.Name)
		}
	}
}
