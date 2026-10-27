// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Command tool converts a key at package level, a finding a declaration names
// by the variable the initializer belongs to.
package main

import (
	"fmt"

	"example.com/findings/internal/i18n"
)

var key = i18n.Key("flow.pair")

func main() {
	fmt.Println(i18n.New(key, i18n.S("first", "a"), i18n.S("second", "b")).Text(nil).String())
}
