package mrapprovalsettings

// PublishedActionIDs exposes the canonical action IDs this package hands a
// model so the catalog test, which must live in the external test package to
// import the catalog builder without a cycle, can hold them against the
// catalog.
var PublishedActionIDs = publishedActionIDs
