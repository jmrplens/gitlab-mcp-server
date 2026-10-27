// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build never

package flows

import "example.com/findings/internal/i18n"

// hidden is left out by its build constraint, so nothing in it is judged.
var hidden = i18n.Verbatim("hidden")
