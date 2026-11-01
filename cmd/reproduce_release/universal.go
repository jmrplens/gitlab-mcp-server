// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"debug/macho"
	"encoding/binary"
	"fmt"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/releaseassets"
)

// The universal (fat) Mach-O format as GoReleaser writes it
// (internal/pipe/universalbinary, after github.com/randall77/makefat): a
// big-endian header of the magic number and the slice count, five 32-bit
// words per slice (cpu type, cpu subtype, offset, size, alignment as a power
// of two), then each slice at the next multiple of 2^14, with zeros between.
// releaseassets.UniversalOrder reads the header back; this is the layout a
// rebuild writes.
const (
	fatAlignBits = 14
	fatAlignment = 1 << fatAlignBits
)

// assembleFat joins the rebuilt darwin binaries into a universal binary with
// its slices in order, laid out as GoReleaser lays it out.
func assembleFat(order []releaseassets.MachOArch, thins [][]byte) ([]byte, error) {
	if len(order) != len(thins) {
		return nil, fmt.Errorf("the published universal binary holds %d architectures and %d darwin binaries were rebuilt", len(order), len(thins))
	}
	byArch := make(map[releaseassets.MachOArch][]byte, len(thins))
	for _, data := range thins {
		arch, err := releaseassets.ThinArch(data)
		if err != nil {
			return nil, fmt.Errorf("a rebuilt darwin binary %w", err)
		}
		if _, seen := byArch[arch]; seen {
			return nil, fmt.Errorf("two rebuilt darwin binaries are for cpu %d subtype %d", arch.CPU, arch.SubCPU)
		}
		byArch[arch] = data
	}

	out := binary.BigEndian.AppendUint32(nil, macho.MagicFat)
	out = binary.BigEndian.AppendUint32(out, uint32(len(order))) //#nosec G115 -- a header word, as GoReleaser writes it, for the two darwin architectures
	offsets := make([]int, len(order))
	offset := fatAlignment
	for i, arch := range order {
		data, ok := byArch[arch]
		if !ok {
			return nil, fmt.Errorf("the published universal binary holds cpu %d subtype %d, which no rebuilt darwin binary is for", arch.CPU, arch.SubCPU)
		}
		offsets[i] = offset
		// The offset and the size are 32-bit words in the format, and
		// GoReleaser converts them the same way, so a value that did not fit
		// would be written as GoReleaser writes it.
		for _, word := range []uint32{uint32(arch.CPU), arch.SubCPU, uint32(offset), uint32(len(data)), fatAlignBits} { //#nosec G115 -- see above
			out = binary.BigEndian.AppendUint32(out, word)
		}
		offset = (offset + len(data) + fatAlignment - 1) / fatAlignment * fatAlignment
	}
	for i, arch := range order {
		out = append(out, make([]byte, offsets[i]-len(out))...)
		out = append(out, byArch[arch]...)
	}
	return out, nil
}
