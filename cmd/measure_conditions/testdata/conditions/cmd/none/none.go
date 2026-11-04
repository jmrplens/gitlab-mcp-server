// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Package none holds no condition gobco counts in either mode, though it
// writes a comparison in a constant, which gobco does not descend into, a
// range, a for with no condition and a select.
package none

// Big compares in a constant declaration.
const Big = 1<<10 > 512

// Wait receives from each of done, and then from tick until stop is
// received from.
func Wait(done []chan struct{}, stop, tick chan struct{}) {
	for _, d := range done {
		<-d
	}
	for {
		select {
		case <-stop:
			return
		case <-tick:
		}
	}
}
