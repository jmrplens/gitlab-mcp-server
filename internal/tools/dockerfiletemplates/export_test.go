package dockerfiletemplates

// PublishedActionIDs is every canonical catalog ID this package hands a model
// through the RelatedActions metadata on its specs, named by constant rather
// than by repeated value. It exists so the external test can hold each of them
// against the built catalog, which this package cannot import itself:
// internal/tools imports this package, so a test in package
// dockerfiletemplates that reached for the catalog would be a cycle.
var PublishedActionIDs = []string{
	actionDockerfileList,
	actionDockerfileGet,
}
