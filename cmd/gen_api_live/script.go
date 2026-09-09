package main

import _ "embed"

// introspectScript is the Ruby this command runs inside the container.
//
// Embedded rather than copied in at build time so the script a maintainer
// reads and the script that produced the record are the same file, and so a
// change to it is a change to this command's diff rather than to a fixture
// somebody has to remember to ship.
//
//go:embed introspect.rb
var introspectScript string
