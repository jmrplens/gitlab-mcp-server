package featureflags

// PublishedActionIDs is every canonical catalog ID this package hands a model:
// the RelatedActions metadata on its specs and the HintAction calls in
// markdown.go both read the same constants, and this slice names those
// constants rather than repeating their values. It exists so the external test
// can hold each of them against the built catalog, which this package cannot
// import itself: internal/tools imports this package, so a test in package
// featureflags that reached for the catalog would be a cycle.
var PublishedActionIDs = []string{
	actionFeatureFlagList,
	actionFeatureFlagGet,
	actionFeatureFlagCreate,
	actionFeatureFlagUpdate,
	actionFeatureFlagDelete,
	actionFeatureFlagUserListGet,
}
