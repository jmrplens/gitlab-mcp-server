// export_test.go exposes the package's canonical action ID block to the
// external catalog test, which cannot see package-local constants.
package mrcontextcommits

// PublishedActionIDs returns every canonical catalog action ID this package
// names to a model, whether through an ActionSpec's RelatedActions or through
// a hint one of its Markdown formatters writes.
//
// It is built from the same constants the package itself reads, so a test over
// it judges what is served rather than a copy of it; listing a literal here
// would prove only that this file and the catalog agree with each other.
func PublishedActionIDs() []string {
	return []string{
		actionContextCommitsList,
		actionContextCommitsCreate,
		actionContextCommitsDelete,
		actionMergeRequestGet,
		actionMergeRequestList,
		actionMergeRequestCommits,
		actionCommitGet,
		actionCommitList,
	}
}
