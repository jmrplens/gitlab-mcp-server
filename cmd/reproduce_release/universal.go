package main

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// The universal (fat) Mach-O format as GoReleaser writes it
// (internal/pipe/universalbinary, after github.com/randall77/makefat): a
// big-endian header of the magic number and the slice count, five 32-bit
// words per slice (cpu type, cpu subtype, offset, size, alignment as a power
// of two), then each slice at the next multiple of 2^14, with zeros between.
const (
	fatMagic      = 0xcafebabe
	fatAlignBits  = 14
	fatAlignment  = 1 << fatAlignBits
	fatHeaderSize = 8
	fatArchSize   = 20
	// thinMagic is the magic number of a 64-bit Mach-O file (MH_MAGIC_64),
	// which a little-endian file writes first.
	thinMagic = 0xfeedfacf
)

// fatArch is what identifies a slice of a universal binary: the cpu type
// and subtype its Mach-O header names.
type fatArch struct {
	cpu, subcpu uint32
}

// readFatOrder reads the slices a universal binary's header lists, in the
// order it lists them.
//
// The order is the one fact about a universal binary its inputs do not fix:
// GoReleaser joins the darwin binaries in the order their builds finished,
// which differs from one run to the next. The published header is therefore
// where a rebuild takes it from, and every other byte of the file is held to
// the rebuilt slices.
func readFatOrder(data []byte) ([]fatArch, error) {
	if len(data) < fatHeaderSize || binary.BigEndian.Uint32(data) != fatMagic {
		return nil, errors.New("is not a universal Mach-O file")
	}
	count := uint64(binary.BigEndian.Uint32(data[4:]))
	if uint64(len(data)) < fatHeaderSize+fatArchSize*count {
		return nil, fmt.Errorf("declares %d architectures and ends inside their table", count)
	}
	order := make([]fatArch, count)
	for i := range order {
		entry := data[fatHeaderSize+fatArchSize*i:]
		order[i] = fatArch{cpu: binary.BigEndian.Uint32(entry), subcpu: binary.BigEndian.Uint32(entry[4:])}
	}
	return order, nil
}

// thinArch reads the cpu type and subtype of a 64-bit Mach-O file, from the
// bytes GoReleaser reads them from.
func thinArch(data []byte) (fatArch, error) {
	if len(data) < 12 || binary.LittleEndian.Uint32(data) != thinMagic {
		return fatArch{}, errors.New("is not a 64-bit Mach-O file")
	}
	return fatArch{cpu: binary.LittleEndian.Uint32(data[4:]), subcpu: binary.LittleEndian.Uint32(data[8:])}, nil
}

// assembleFat joins the rebuilt darwin binaries into a universal binary with
// its slices in order, laid out as GoReleaser lays it out.
func assembleFat(order []fatArch, thins [][]byte) ([]byte, error) {
	if len(order) != len(thins) {
		return nil, fmt.Errorf("the published universal binary holds %d architectures and %d darwin binaries were rebuilt", len(order), len(thins))
	}
	byArch := make(map[fatArch][]byte, len(thins))
	for _, data := range thins {
		arch, err := thinArch(data)
		if err != nil {
			return nil, fmt.Errorf("a rebuilt darwin binary %w", err)
		}
		if _, seen := byArch[arch]; seen {
			return nil, fmt.Errorf("two rebuilt darwin binaries are for cpu %d subtype %d", arch.cpu, arch.subcpu)
		}
		byArch[arch] = data
	}

	out := binary.BigEndian.AppendUint32(nil, fatMagic)
	out = binary.BigEndian.AppendUint32(out, uint32(len(order))) //#nosec G115 -- a header word, as GoReleaser writes it, for the two darwin architectures
	offsets := make([]int, len(order))
	offset := fatAlignment
	for i, arch := range order {
		data, ok := byArch[arch]
		if !ok {
			return nil, fmt.Errorf("the published universal binary holds cpu %d subtype %d, which no rebuilt darwin binary is for", arch.cpu, arch.subcpu)
		}
		offsets[i] = offset
		// The offset and the size are 32-bit words in the format, and
		// GoReleaser converts them the same way, so a value that did not fit
		// would be written as GoReleaser writes it.
		for _, word := range []uint32{arch.cpu, arch.subcpu, uint32(offset), uint32(len(data)), fatAlignBits} { //#nosec G115 -- see above
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
