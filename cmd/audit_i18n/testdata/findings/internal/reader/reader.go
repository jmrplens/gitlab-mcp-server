// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package reader

import "example.com/findings/internal/i18n"

// Excused converts a key it read from source, under a declaration.
func Excused(s string) i18n.Key {
	return i18n.Key(s)
}

// Odd converts a key under a declaration whose category nobody defined.
func Odd(s string) i18n.Key {
	return (i18n.Key)(s)
}

// Stale no longer converts anything, so its declaration excuses nothing.
func Stale(s string) string {
	return s
}
