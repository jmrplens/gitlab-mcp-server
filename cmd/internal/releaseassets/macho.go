// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package releaseassets

import (
	"debug/macho"
	"encoding/binary"
	"errors"
	"fmt"
)

// The header words of the universal (fat) Mach-O format as GoReleaser writes
// it (internal/pipe/universalbinary, after github.com/randall77/makefat): a
// big-endian header of the magic number and the slice count, then five
// 32-bit words per slice, the cpu type, the cpu subtype, the offset, the size
// and the alignment, which are debug/macho's FatArchHeader.
const (
	fatHeaderSize = 8
	fatArchSize   = 20
	// thinHeaderWords is how much of a thin file's header the cpu type and
	// subtype are read from: the magic number, then the two words.
	thinHeaderWords = 12
)

// MachOArch is what identifies an image of a universal binary: the cpu type
// and subtype its Mach-O header names, which the universal header lists it
// by.
type MachOArch struct {
	CPU    macho.Cpu
	SubCPU uint32
}

// UniversalOrder reads the slices a universal binary's header lists, in the
// order it lists them.
//
// The order is the one fact about a universal binary its inputs do not fix:
// GoReleaser joins the darwin binaries in the order their builds finished,
// which differs from one run to the next. The published header is therefore
// where a rebuild takes it from. Only the header is read, so a header that
// lists nothing is an empty order and the slices are not looked at: every
// other byte of the file is for the rebuild to match.
func UniversalOrder(data []byte) ([]MachOArch, error) {
	if len(data) < fatHeaderSize || binary.BigEndian.Uint32(data) != macho.MagicFat {
		return nil, errors.New("is not a universal Mach-O file")
	}
	count := uint64(binary.BigEndian.Uint32(data[4:]))
	if uint64(len(data)) < fatHeaderSize+fatArchSize*count {
		return nil, fmt.Errorf("declares %d architectures and ends inside their table", count)
	}
	order := make([]MachOArch, count)
	for i := range order {
		entry := data[fatHeaderSize+fatArchSize*i:]
		order[i] = MachOArch{CPU: macho.Cpu(binary.BigEndian.Uint32(entry)), SubCPU: binary.BigEndian.Uint32(entry[4:])}
	}
	return order, nil
}

// ThinArch reads the cpu type and subtype of a 64-bit little-endian Mach-O
// file, from the words GoReleaser reads them from when it joins the file into
// a universal one.
func ThinArch(data []byte) (MachOArch, error) {
	if len(data) < thinHeaderWords || binary.LittleEndian.Uint32(data) != macho.Magic64 {
		return MachOArch{}, errors.New("is not a 64-bit Mach-O file")
	}
	return MachOArch{CPU: macho.Cpu(binary.LittleEndian.Uint32(data[4:])), SubCPU: binary.LittleEndian.Uint32(data[8:])}, nil
}
