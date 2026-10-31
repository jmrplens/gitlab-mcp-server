// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package releaseassets

import (
	"bytes"
	"debug/macho"
	"encoding/binary"
	"reflect"
	"testing"
)

// The two darwin architectures, as <mach/machine.h> spells them:
// CPU_TYPE_X86_64 with CPU_SUBTYPE_X86_64_ALL, and CPU_TYPE_ARM64 with
// CPU_SUBTYPE_ARM64_ALL.
var (
	darwinAMD64 = MachOArch{CPU: macho.CpuAmd64, SubCPU: 3}
	darwinARM64 = MachOArch{CPU: macho.CpuArm64, SubCPU: 0}
)

// syntheticThin is a stand-in for a 64-bit Mach-O file of size bytes: the
// magic number and the cpu type GoReleaser reads, then a pattern, which is
// all the header words are read from.
func syntheticThin(arch MachOArch, size int, fill byte) []byte {
	data := bytes.Repeat([]byte{fill}, size)
	binary.LittleEndian.PutUint32(data, 0xfeedfacf)
	binary.LittleEndian.PutUint32(data[4:], uint32(arch.CPU))
	binary.LittleEndian.PutUint32(data[8:], arch.SubCPU)
	return data
}

// goreleaserFat joins thin Mach-O files the way GoReleaser v2.19.0 does
// (internal/pipe/universalbinary, makeUniversalBinary), in the order given: a
// big-endian fat header, each slice at the next multiple of 2^14, zeros in
// between. It is written out here from that source rather than shared with
// the code under test, so the two can disagree.
func goreleaserFat(thins ...[]byte) []byte {
	const alignBits = 14
	const align = 1 << alignBits
	header := []uint32{0xcafebabe, uint32(len(thins))} //#nosec G115 -- a slice count of a test fixture, at most two
	offsets := make([]int, len(thins))
	offset := align
	for i, data := range thins {
		offsets[i] = offset
		cpu, subcpu := binary.LittleEndian.Uint32(data[4:8]), binary.LittleEndian.Uint32(data[8:12])
		header = append(header, cpu, subcpu, uint32(offset), uint32(len(data)), alignBits) //#nosec G115 -- offsets and sizes of fixture slices of a few kilobytes
		offset = (offset + len(data) + align - 1) / align * align
	}
	var out bytes.Buffer
	_ = binary.Write(&out, binary.BigEndian, header)
	for i, data := range thins {
		out.Write(make([]byte, offsets[i]-out.Len()))
		out.Write(data)
	}
	return out.Bytes()
}

// TestUniversalOrder_ReadsTheHeader covers a header read back in its own
// order, and every file whose header cannot be read: too short to hold one,
// another magic number, and an architecture table the file ends inside.
func TestUniversalOrder_ReadsTheHeader(t *testing.T) {
	t.Parallel()

	fat := goreleaserFat(syntheticThin(darwinARM64, 32, 1), syntheticThin(darwinAMD64, 32, 2))
	// The header ends where its table does, which is all UniversalOrder
	// reads, and a header listing nothing is eight bytes and an empty order.
	for _, tc := range []struct {
		name string
		data []byte
		want []MachOArch
	}{
		{name: "a whole file", data: fat, want: []MachOArch{darwinARM64, darwinAMD64}},
		{name: "the header alone", data: fat[:8+20*2], want: []MachOArch{darwinARM64, darwinAMD64}},
		{name: "a header listing nothing", data: []byte{0xca, 0xfe, 0xba, 0xbe, 0, 0, 0, 0}, want: []MachOArch{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got, err := UniversalOrder(tc.data); err != nil || !reflect.DeepEqual(got, tc.want) {
				t.Errorf("UniversalOrder = %v, %v, want %v", got, err, tc.want)
			}
		})
	}
	for _, tc := range []struct {
		name string
		data []byte
		want string
	}{
		{name: "too short", data: fat[:7], want: "is not a universal Mach-O file"},
		{name: "a thin Mach-O file", data: syntheticThin(darwinAMD64, 64, 0), want: "is not a universal Mach-O file"},
		{name: "a truncated table", data: fat[:8+20+19], want: "declares 2 architectures and ends inside their table"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, refused := UniversalOrder(tc.data)
			if refused == nil || refused.Error() != tc.want {
				t.Errorf("UniversalOrder = %v, want the error %q", refused, tc.want)
			}
			if got != nil {
				t.Errorf("UniversalOrder refused the file and returned %v, want nothing", got)
			}
		})
	}
}

// TestThinArch_ReadsTheCPUType covers a thin file's cpu type and subtype, read
// where GoReleaser reads them, and the files that have none: one too short, a
// universal file, and a 32-bit one.
func TestThinArch_ReadsTheCPUType(t *testing.T) {
	t.Parallel()

	if got, err := ThinArch(syntheticThin(darwinARM64, 12, 0)); err != nil || got != darwinARM64 {
		t.Errorf("ThinArch = %v, %v, want %v", got, err, darwinARM64)
	}
	thin32 := syntheticThin(darwinAMD64, 64, 0)
	binary.LittleEndian.PutUint32(thin32, macho.Magic32)
	for name, data := range map[string][]byte{
		"too short":               syntheticThin(darwinARM64, 12, 0)[:11],
		"a universal Mach-O file": goreleaserFat(syntheticThin(darwinARM64, 12, 0)),
		"a 32-bit Mach-O file":    thin32,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := ThinArch(data)
			if err == nil || err.Error() != "is not a 64-bit Mach-O file" {
				t.Errorf("ThinArch of %d bytes that are not a thin Mach-O file = %v", len(data), err)
			}
			if got != (MachOArch{}) {
				t.Errorf("ThinArch refused the file and returned %v, want nothing", got)
			}
		})
	}
}

// TestUniversalOrder_ReadsWhatThinArchReads holds the two readers to each
// other: a universal file's header names each slice by the cpu type and
// subtype the slice's own header carries.
func TestUniversalOrder_ReadsWhatThinArchReads(t *testing.T) {
	t.Parallel()

	thins := [][]byte{syntheticThin(darwinAMD64, 40, 3), syntheticThin(darwinARM64, 40, 4)}
	order, err := UniversalOrder(goreleaserFat(thins...))
	if err != nil {
		t.Fatalf("UniversalOrder: %v", err)
	}
	for i, data := range thins {
		got, thinErr := ThinArch(data)
		if thinErr != nil || got != order[i] {
			t.Errorf("slice %d: ThinArch = %v, %v, and the header lists %v", i, got, thinErr, order[i])
		}
	}
}
