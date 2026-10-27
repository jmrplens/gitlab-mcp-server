// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Command tool renders one text, so the default patterns find a command too.
package main

import (
	"context"
	"fmt"

	"example.com/clean/internal/flows"
)

func main() {
	fmt.Println(flows.Greet(context.Background(), "world").String())
}
