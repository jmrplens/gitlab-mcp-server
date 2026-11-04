// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package docblocks

import "errors"

// The ways a page can fail to give a test the block it asked for. Each is
// wrapped with the page, the key and the lines involved, and matched with
// [errors.Is].
var (
	// ErrUnknownKey is a key no fence of the English page carries.
	ErrUnknownKey = errors.New("no fence carries the key")
	// ErrDuplicateKey is a key two fences of one page carry.
	ErrDuplicateKey = errors.New("more than one fence carries the key")
	// ErrNotInTwin is a key the English page carries and its Spanish twin
	// does not.
	ErrNotInTwin = errors.New("the Spanish twin carries no fence with the key")
	// ErrUnclosedFence is a fence the page never closes.
	ErrUnclosedFence = errors.New("the fence is never closed")
	// ErrBadKey is an e2e option whose value is not a key, or a fence that
	// names two.
	ErrBadKey = errors.New("the fence's e2e option is not one key")
)

// Block is one fenced code block of a page.
type Block struct {
	// Lang is the first word of the fence's info string.
	Lang string
	// Info is the info string after the language, as the page wrote it.
	Info string
	// Key is the value of the fence's e2e option, empty when it has none.
	Key string
	// Body is the content between the fences, each line ending in a newline
	// and stripped of the opening fence's indentation.
	Body string
	// Line is the 1-based line of the opening fence.
	Line int
}

// Page is an English page of the site, read together with its Spanish twin.
type Page struct {
	// Path is the English page's file.
	Path string
	// Twin is the Spanish page's file.
	Twin string

	blocks []Block
	twin   []Block
}

// Parse returns the fenced code blocks of a page's source, in page order.
func Parse(src []byte) ([]Block, error) {
	return nil, errors.New("not implemented")
}

// Open reads the English page rel under docsDir and its twin under
// docsDir/es, and parses both.
func Open(docsDir, rel string) (*Page, error) {
	return nil, errors.New("not implemented")
}

// Block returns the one block of the English page whose fence carries
// e2e=key, after checking that exactly one fence of the Spanish twin carries
// it too.
func (p *Page) Block(key string) (Block, error) {
	return Block{}, errors.New("not implemented")
}
