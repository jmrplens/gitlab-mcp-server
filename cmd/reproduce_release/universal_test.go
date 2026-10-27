// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// universal_test.go covers the universal darwin binary: the order its header
// lists its slices in, and the file joined from rebuilt darwin binaries in
// that order, laid out the way GoReleaser lays it out.
package main

import (
	"bytes"
	"encoding/binary"
	"reflect"
	"strings"
	"testing"
)

// The cpu types of the two darwin architectures, as <mach/machine.h> spells
// them: CPU_TYPE_X86_64 with CPU_SUBTYPE_X86_64_ALL, and CPU_TYPE_ARM64 with
// CPU_SUBTYPE_ARM64_ALL.
var (
	darwinAMD64 = fatArch{cpu: 0x01000007, subcpu: 3}
	darwinARM64 = fatArch{cpu: 0x0100000c, subcpu: 0}
)

// syntheticThin is a stand-in for a 64-bit Mach-O file of size bytes: the
// magic number and the cpu type GoReleaser reads, then a pattern, which is
// all the joining reads of it.
func syntheticThin(arch fatArch, size int, fill byte) []byte {
	data := bytes.Repeat([]byte{fill}, size)
	binary.LittleEndian.PutUint32(data, 0xfeedfacf)
	binary.LittleEndian.PutUint32(data[4:], arch.cpu)
	binary.LittleEndian.PutUint32(data[8:], arch.subcpu)
	return data
}

// goreleaserFat joins thin Mach-O files the way GoReleaser v2.18.3 does
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

// TestAssembleFat_LaysTheSlicesOutAsGoReleaserDoes joins two slices in the
// order a published header gives, the reverse of the order they are handed
// in, and holds the result to GoReleaser's layout byte for byte.
func TestAssembleFat_LaysTheSlicesOutAsGoReleaserDoes(t *testing.T) {
	t.Parallel()

	// The second case has a slice exactly 2^14 bytes long, where rounding the
	// next offset up to the alignment is easiest to get one alignment wrong.
	for _, tc := range []struct {
		name         string
		amd64, arm64 int
	}{
		{name: "slices of any length", amd64: 20000, arm64: 5000},
		{name: "a slice of exactly the alignment", amd64: 1 << 14, arm64: 5000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			amd64 := syntheticThin(darwinAMD64, tc.amd64, 0xaa)
			arm64 := syntheticThin(darwinARM64, tc.arm64, 0xbb)
			got, err := assembleFat([]fatArch{darwinAMD64, darwinARM64}, [][]byte{arm64, amd64})
			if err != nil {
				t.Fatalf("assembleFat: %v", err)
			}
			if want := goreleaserFat(amd64, arm64); !bytes.Equal(got, want) {
				t.Errorf("assembleFat is %d bytes and GoReleaser's layout %d: %s", len(got), len(want), firstDifference(got, want, againstItself))
			}
		})
	}
}

// TestAssembleFat_RefusesSlicesThatDoNotFitTheHeader covers every way the
// rebuilt darwin binaries can fail to make the file the header describes.
func TestAssembleFat_RefusesSlicesThatDoNotFitTheHeader(t *testing.T) {
	t.Parallel()

	amd64 := syntheticThin(darwinAMD64, 64, 1)
	arm64 := syntheticThin(darwinARM64, 64, 2)
	for _, tc := range []struct {
		name  string
		order []fatArch
		thins [][]byte
		want  string
	}{
		{name: "a slice too few", order: []fatArch{darwinAMD64, darwinARM64}, thins: [][]byte{amd64}, want: "holds 2 architectures and 1 darwin binaries were rebuilt"},
		{name: "a file that is not Mach-O", order: []fatArch{darwinAMD64}, thins: [][]byte{[]byte("not a binary")}, want: "is not a 64-bit Mach-O file"},
		{name: "two slices for one architecture", order: []fatArch{darwinAMD64, darwinARM64}, thins: [][]byte{amd64, amd64}, want: "two rebuilt darwin binaries are for cpu 16777223 subtype 3"},
		{name: "an architecture no slice is for", order: []fatArch{darwinARM64}, thins: [][]byte{amd64}, want: "holds cpu 16777228 subtype 0, which no rebuilt darwin binary is for"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if _, err := assembleFat(tc.order, tc.thins); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("assembleFat = %v, want an error containing %q", err, tc.want)
			}
		})
	}
	if _, err := assembleFat([]fatArch{darwinARM64, darwinAMD64}, [][]byte{amd64, arm64}); err != nil {
		t.Errorf("assembleFat of two slices that fit: %v", err)
	}
}

// TestReadFatOrder_ReadsTheHeader covers a header read back in its own order,
// and every file whose header cannot be read: too short to hold one, another
// magic number, and an architecture table the file ends inside.
func TestReadFatOrder_ReadsTheHeader(t *testing.T) {
	t.Parallel()

	fat := goreleaserFat(syntheticThin(darwinARM64, 32, 1), syntheticThin(darwinAMD64, 32, 2))
	// The header ends where its table does, which is all readFatOrder reads,
	// and a header listing nothing is eight bytes and an empty order.
	for _, tc := range []struct {
		name string
		data []byte
		want []fatArch
	}{
		{name: "a whole file", data: fat, want: []fatArch{darwinARM64, darwinAMD64}},
		{name: "the header alone", data: fat[:8+20*2], want: []fatArch{darwinARM64, darwinAMD64}},
		{name: "a header listing nothing", data: []byte{0xca, 0xfe, 0xba, 0xbe, 0, 0, 0, 0}, want: []fatArch{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got, err := readFatOrder(tc.data); err != nil || !reflect.DeepEqual(got, tc.want) {
				t.Errorf("readFatOrder = %v, %v, want %v", got, err, tc.want)
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

			if _, refused := readFatOrder(tc.data); refused == nil || !strings.Contains(refused.Error(), tc.want) {
				t.Errorf("readFatOrder = %v, want an error containing %q", refused, tc.want)
			}
		})
	}
}

// TestThinArch_ReadsTheCPUType covers a thin file's cpu type and subtype, read
// where GoReleaser reads them, and the two files that have none: one too short
// and one with another magic number.
func TestThinArch_ReadsTheCPUType(t *testing.T) {
	t.Parallel()

	if got, err := thinArch(syntheticThin(darwinARM64, 12, 0)); err != nil || got != darwinARM64 {
		t.Errorf("thinArch = %v, %v, want %v", got, err, darwinARM64)
	}
	for name, data := range map[string][]byte{
		"too short":               syntheticThin(darwinARM64, 12, 0)[:11],
		"a universal Mach-O file": goreleaserFat(syntheticThin(darwinARM64, 12, 0)),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if _, err := thinArch(data); err == nil || !strings.Contains(err.Error(), "is not a 64-bit Mach-O file") {
				t.Errorf("thinArch of %d bytes that are not a thin Mach-O file = %v", len(data), err)
			}
		})
	}
}
