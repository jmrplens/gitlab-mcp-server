//go:build e2e

// tags_test.go covers a tag through the server: its lifecycle, its
// protection and the removal of it, and the signature an unsigned tag has
// none of, which the old suite called and never read the answer of.

package common

import (
	"slices"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/tags"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// tagNames returns the names of a tag listing.
func tagNames(listed []tags.Output) []string {
	names := make([]string, 0, len(listed))
	for _, tag := range listed {
		names = append(names, tag.Name)
	}
	return names
}

// protectedTagNames returns the names of a protected tag listing.
func protectedTagNames(listed []tags.ProtectedTagOutput) []string {
	names := make([]string, 0, len(listed))
	for _, tag := range listed {
		names = append(names, tag.Name)
	}
	return names
}

// createTag creates an annotated tag on the default branch of a project
// through the server and checks the answer names it.
func createTag(e *harness.Env, s *harness.Session, project fixture.Project, name string) tags.Output {
	e.T.Helper()

	created := harness.Do[tags.Output](s, actionTagCreate, map[string]any{
		"project_id": project.IDParam(), "tag_name": name, "ref": project.DefaultBranch, "message": "e2e tag " + name,
	})
	if created.Name != name || created.Target == "" {
		e.T.Fatalf("tag create answered %+v, want %s with a target", created, name)
	}
	return created
}

// TestTag_Lifecycle_CreateGetListDelete creates an annotated tag per
// surface on a shared project, reads it back, finds it in the listing,
// deletes it and asserts the read afterwards is refused as not found.
//
// Replaces: TestIndividual_Tags, TestMeta_Tags
func TestTag_Lifecycle_CreateGetListDelete(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("tags"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		name := e.Name("v0.1.0")
		params := map[string]any{"project_id": project.IDParam(), "tag_name": name}

		created := createTag(e, s, project, name)

		got := harness.Do[tags.Output](s, actionTagGet, params)
		if got.Name != name || got.Target != created.Target || got.Message != "e2e tag "+name {
			e.T.Errorf("tag get answered %+v, want %s at %s with its message", got, name, created.Target)
		}
		listed := harness.Do[tags.ListOutput](s, actionTagList, map[string]any{"project_id": project.IDParam()})
		if !slices.Contains(tagNames(listed.Tags), name) {
			e.T.Errorf("the tag listing does not hold %s: %v", name, tagNames(listed.Tags))
		}

		harness.DoVoid(s, actionTagDelete, params)
		refused := harness.Refused(s, actionTagGet, params, harness.FailureNotFound)
		e.T.Logf("the read after the delete is refused: %s", firstLine(refused))
	})
}

// TestTag_Protection_ProtectGetUnprotectAndUnsignedSignature creates a tag
// per surface, protects it, finds the protection in the listing and reads
// it, asks for the signature GitLab answers 404 for on a tag nobody signed,
// removes the protection, asserts the protected read is then refused, and
// deletes the tag.
//
// Replaces: TestMeta_TagsProtected, TestMeta_ProtectedTags
func TestTag_Protection_ProtectGetUnprotectAndUnsignedSignature(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("prottags"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		name := e.Name("prot")
		params := map[string]any{"project_id": project.IDParam(), "tag_name": name}

		createTag(e, s, project, name)

		protected := harness.Do[tags.ProtectedTagOutput](s, actionTagProtect, params)
		if protected.Name != name || len(protected.CreateAccessLevels) == 0 {
			e.T.Fatalf("tag protect answered %+v, want %s with a create access level", protected, name)
		}
		listed := harness.Do[tags.ListProtectedTagsOutput](s, actionTagListProtected, map[string]any{"project_id": project.IDParam()})
		if !slices.Contains(protectedTagNames(listed.Tags), name) {
			e.T.Errorf("the protected tag listing does not hold %s: %v", name, protectedTagNames(listed.Tags))
		}
		got := harness.Do[tags.ProtectedTagOutput](s, actionTagGetProtected, params)
		if got.Name != name {
			e.T.Errorf("protected tag get answered %+v, want %s", got, name)
		}

		refused := harness.Refused(s, actionTagGetSignature, params, harness.FailureNotFound)
		assertMentions(e, "the signature read of an unsigned tag", refused, "signed")

		harness.DoVoid(s, actionTagUnprotect, params)
		unprotected := harness.Refused(s, actionTagGetProtected, params, harness.FailureNotFound)
		e.T.Logf("the protected read after the unprotect is refused: %s", firstLine(unprotected))

		harness.DoVoid(s, actionTagDelete, params)
		gone := harness.Refused(s, actionTagGet, params, harness.FailureNotFound)
		e.T.Logf("the read after the delete is refused: %s", firstLine(gone))
	})
}
