// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// universal_test.go covers the universal darwin binary joined from rebuilt
// darwin binaries in the order the published one's header lists, laid out
// the way GoReleaser lays it out. Reading that order, and a thin file's cpu
// type, is cmd/internal/releaseassets', and its cases are there.
package main

import (
	"bytes"
	"encoding/binary"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/releaseassets"
)

// The cpu types of the two darwin architectures, as <mach/machine.h> spells
// them: CPU_TYPE_X86_64 with CPU_SUBTYPE_X86_64_ALL, and CPU_TYPE_ARM64 with
// CPU_SUBTYPE_ARM64_ALL.
var (
	darwinAMD64 = releaseassets.MachOArch{CPU: 0x01000007, SubCPU: 3}
	darwinARM64 = releaseassets.MachOArch{CPU: 0x0100000c, SubCPU: 0}
)

// syntheticThin is a stand-in for a 64-bit Mach-O file of size bytes: the
// magic number and the cpu type GoReleaser reads, then a pattern, which is
// all the joining reads of it.
func syntheticThin(arch releaseassets.MachOArch, size int, fill byte) []byte {
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
			got, err := assembleFat([]releaseassets.MachOArch{darwinAMD64, darwinARM64}, [][]byte{arm64, amd64})
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
		order []releaseassets.MachOArch
		thins [][]byte
		want  string
	}{
		{name: "a slice too few", order: []releaseassets.MachOArch{darwinAMD64, darwinARM64}, thins: [][]byte{amd64}, want: "holds 2 architectures and 1 darwin binaries were rebuilt"},
		{name: "a file that is not Mach-O", order: []releaseassets.MachOArch{darwinAMD64}, thins: [][]byte{[]byte("not a binary")}, want: "is not a 64-bit Mach-O file"},
		{name: "two slices for one architecture", order: []releaseassets.MachOArch{darwinAMD64, darwinARM64}, thins: [][]byte{amd64, amd64}, want: "two rebuilt darwin binaries are for cpu 16777223 subtype 3"},
		{name: "an architecture no slice is for", order: []releaseassets.MachOArch{darwinARM64}, thins: [][]byte{amd64}, want: "holds cpu 16777228 subtype 0, which no rebuilt darwin binary is for"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if _, err := assembleFat(tc.order, tc.thins); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("assembleFat = %v, want an error containing %q", err, tc.want)
			}
		})
	}
	if _, err := assembleFat([]releaseassets.MachOArch{darwinARM64, darwinAMD64}, [][]byte{amd64, arm64}); err != nil {
		t.Errorf("assembleFat of two slices that fit: %v", err)
	}
}

// TestAssembleFat_JoinsWhatUniversalOrderReadsBack holds the writer to the
// reader the published header is read with: a file joined in an order lists
// that order.
func TestAssembleFat_JoinsWhatUniversalOrderReadsBack(t *testing.T) {
	t.Parallel()

	order := []releaseassets.MachOArch{darwinARM64, darwinAMD64}
	joined, err := assembleFat(order, [][]byte{syntheticThin(darwinAMD64, 64, 1), syntheticThin(darwinARM64, 64, 2)})
	if err != nil {
		t.Fatalf("assembleFat: %v", err)
	}
	got, err := releaseassets.UniversalOrder(joined)
	if err != nil || !slices.Equal(got, order) {
		t.Errorf("UniversalOrder of the joined file = %v, %v, want %v", got, err, order)
	}
}
