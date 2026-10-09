// The release numbers a page keeps literal in a shape that usually means "the
// release you install today", each with the reason it is not the current one.
//
// site/scripts/check-version-token.mjs fails on an install command, image tag,
// package pin, download URL, sample `--version` or /health output, or "the
// current release is" sentence that writes a number instead of %%VERSION%%.
// Most numbers in those shapes should be the token. The ones here should not:
// they record a measurement taken on one release, or a condition that names
// one release and would turn false if it named the next.
//
// `page` is the path under site/src/content/docs, `text` a piece of one line of
// that page holding the number, so a literal on a line that does not carry the
// same text is still judged. `category` is "measurement" (a fact about that
// release, true for good) or "condition" (true while that release is the
// published one), and `reason` says why. A declaration that matches nothing
// fails the check: a page that moved on leaves no excuse behind for the next
// literal to hide under. So does a condition once server.json names a later
// release, since the sentence it excuses has stopped describing the release
// the site documents. That happens on the commit the release workflow stamps
// server.json with, after the release is out, so prefer a sentence that stays
// true whichever release is the latest ("from the first release after 3.1.0")
// over a condition; there is none today.
export const LITERAL_RELEASES = [
	{
		page: "install/nuget.mdx",
		text: "`dnx gitlab-mcp-server@3.1.0`",
		category: "measurement",
		reason:
			"what dnx did with 3.1.0 in its cache and no network, measured on the 10.0.401 SDK",
	},
	{
		page: "es/install/nuget.mdx",
		text: "`dnx gitlab-mcp-server@3.1.0`",
		category: "measurement",
		reason:
			"what dnx did with 3.1.0 in its cache and no network, measured on the 10.0.401 SDK",
	},
];
