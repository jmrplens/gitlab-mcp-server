package groupimportexport

// PublishedActionIDs is every canonical catalog action ID this package names
// to a model: the block action_specs.go and markdown.go share, which is what
// the RelatedActions of each spec and the next-step hints of each formatter
// are built from.
//
// It is exported for the external test package, which holds each of these to
// the catalog the server really builds. The package's own test cannot do that
// itself: internal/tools imports this package, so a test inside it that
// imported internal/tools back would be an import cycle.
func PublishedActionIDs() []string {
	return []string{
		actionGroupExportSchedule,
		actionGroupExportDownload,
		actionGroupImportFile,
		actionGroupGet,
		actionGroupList,
	}
}

// QualifyActionName prefixes a bare spec name with the catalog domain these
// specs are aggregated under, which is how the IDs above are derived. The
// external test uses it to hold each registered action's own name to the
// catalog, so the derivation cannot drift from what the catalog registers.
func QualifyActionName(name string) string {
	return groupDomain + name
}
