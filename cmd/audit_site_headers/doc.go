// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Command audit_site_headers fetches the documentation site and judges the
// security headers its responses carry: the four the OpenSSF Best Practices
// criterion hardened_site names, Content-Security-Policy,
// Strict-Transport-Security, X-Content-Type-Options as nosniff, and
// X-Frame-Options, each of which must be sent with a value that is not
// permissive.
//
//	audit_site_headers [-url <page>]... [-attempts n] [-wait d] [-annotate] [-summary <file>]
//
// The site is a GitHub Pages build served through Cloudflare, so no file in
// this repository sets a header: whatever the responses carry is set in
// Cloudflare by the maintainer. That is why the check runs after a deploy
// (.github/workflows/pages.yml, the headers job) rather than in CI, and why it
// reads the live responses rather than anything in the tree.
//
// Each header is declared in headers.go with the value to set and a judge of
// what is sent, and all four are declared required: the absence of one, or a
// permissive value, fails the run, since losing it is a regression.
// Content-Security-Policy and X-Frame-Options come from the Cloudflare
// response header rule, and the page that says what it sets is
// docs/development/repository-settings.md, the documentation site response
// headers. A header the site does not serve yet can instead be declared
// pending, with the setting only the maintainer can change that it waits for.
// A pending header is reported with the value to set and never fails the run;
// once it is served, the report asks for it to be declared required, in
// headers.go and in the tests that pin the declarations, so that losing it
// fails the check from then on. None is declared pending.
//
// What a judge refuses:
//
//   - Strict-Transport-Security without a max-age a browser can read, or with
//     one under a year, the least the HSTS preload list accepts. A browser
//     reads the first such header field and ignores the rest.
//   - X-Content-Type-Options other than nosniff.
//   - X-Frame-Options that refuses no frame, read as the HTML standard reads
//     it: DENY or SAMEORIGIN alone, or several values among which one of
//     deny, sameorigin and allowall appears, refuse; ALLOW-FROM and ALLOWALL
//     alone do not.
//   - Content-Security-Policy that governs no script (neither script-src nor
//     default-src), lets a script load from any host (*, a scheme alone such
//     as https: or data:, or a host that is the wildcard) through script-src,
//     default-src in its absence, or script-src-elem, which a CSP Level 3
//     browser reads for a script element in their place, lets a script
//     evaluate text as code ('unsafe-eval'), or lets a plugin load (no
//     object-src or default-src of 'none'). Every policy a response carries
//     is enforced, so one that passes is enough.
//     'unsafe-inline' and 'wasm-unsafe-eval' pass: Starlight writes its theme,
//     sidebar and tab scripts inline, and Pagefind compiles its index from
//     WebAssembly.
//
// With no -url it judges the published site's root. A page is fetched up to
// -attempts times, -wait apart, while it fails or answers 5xx, since a deploy
// can still be propagating when the job starts; any other answer is final. A page that
// cannot be fetched, or answers other than 2xx, fails the run. -annotate adds
// a GitHub Actions workflow command for every line that is not ok, so a
// failure, a pending header and a header now served each show on the run, and
// -summary appends the report as a Markdown table to the file it names, the
// job summary in the workflow.
//
// It exits 0 when every required header of every page holds, 1 when one does
// not, a page was not judged or the summary could not be written, and 2 on a
// usage error.
package main
