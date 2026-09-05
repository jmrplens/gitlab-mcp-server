// Command audit_install_buttons checks the one-click install buttons against
// what the pages around them claim.
//
// A button's configuration travels inside its URL, base64 in every client this
// project links and percent-encoded on top of that in some. Nothing about it is
// visible in review, and no text search finds a flag inside it: removing
// "--http=false" from every example in the tree left eight buttons still
// registering it, because the string never appears as those characters
// anywhere. The one that got fixed first was fixed by hand, and the second
// encoding was missed, twice.
//
// So this audit decodes rather than searches, and holds the buttons to the
// promise the prose makes about them: that every button registers the same
// configuration. Buttons are grouped by the command they launch, since a
// Docker button and an npx button are different configurations on purpose, and
// within a group the arguments have to agree.
package main
