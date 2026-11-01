// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package releaseassets

import (
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"encoding/binary"
	"os"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// machOFile is the smallest 64-bit little-endian Mach-O file debug/macho
// reads: the 32-byte header of an executable with no load commands, for cpu.
func machOFile(cpu macho.Cpu, subcpu uint32) []byte {
	data := make([]byte, 32)
	binary.LittleEndian.PutUint32(data, macho.Magic64)
	binary.LittleEndian.PutUint32(data[4:], uint32(cpu))
	binary.LittleEndian.PutUint32(data[8:], subcpu)
	binary.LittleEndian.PutUint32(data[12:], uint32(macho.TypeExec))
	return data
}

// peFile is the smallest PE file debug/pe reads, for machine: an MZ stub
// whose e_lfanew points at the PE signature right after it, and a COFF file
// header with no sections, symbols or optional header, padded to the 96
// bytes debug/pe reads of a DOS header.
func peFile(machine uint16) []byte {
	const signatureAt = 0x40
	data := make([]byte, 128)
	copy(data, "MZ")
	binary.LittleEndian.PutUint32(data[0x3c:], signatureAt)
	copy(data[signatureAt:], "PE\x00\x00")
	binary.LittleEndian.PutUint16(data[signatureAt+4:], machine)
	return data
}

// TestIdentify_NamesTheTargetOfEachReleaseBinary covers every target the
// release builds, in each format it is published in: ELF for linux, a thin
// Mach-O file for each darwin architecture and the universal one joining
// them, whose order is the file's own, and PE for windows.
func TestIdentify_NamesTheTargetOfEachReleaseBinary(t *testing.T) {
	t.Parallel()

	amd64, arm64 := machOFile(macho.CpuAmd64, 3), machOFile(macho.CpuArm64, 0)
	for _, tc := range []struct {
		name string
		data []byte
		want Binary
	}{
		{name: "linux/amd64", data: newELF(elf.EM_X86_64, glibcAMD64).image(), want: Binary{Format: FormatELF, GOOS: "linux", Arches: []string{"amd64"}}},
		{name: "linux/arm64", data: newELF(elf.EM_AARCH64, glibcARM64).image(), want: Binary{Format: FormatELF, GOOS: "linux", Arches: []string{"arm64"}}},
		{name: "a static linux/amd64", data: newELF(elf.EM_X86_64).image(), want: Binary{Format: FormatELF, GOOS: "linux", Arches: []string{"amd64"}}},
		{name: "darwin/amd64", data: amd64, want: Binary{Format: FormatMachO, GOOS: "darwin", Arches: []string{"amd64"}}},
		{name: "darwin/arm64", data: arm64, want: Binary{Format: FormatMachO, GOOS: "darwin", Arches: []string{"arm64"}}},
		{name: "universal, arm64 first", data: goreleaserFat(arm64, amd64), want: Binary{Format: FormatUniversal, GOOS: "darwin", Arches: []string{"arm64", "amd64"}}},
		{name: "universal, amd64 first", data: goreleaserFat(amd64, arm64), want: Binary{Format: FormatUniversal, GOOS: "darwin", Arches: []string{"amd64", "arm64"}}},
		{name: "windows/amd64", data: peFile(pe.IMAGE_FILE_MACHINE_AMD64), want: Binary{Format: FormatPE, GOOS: "windows", Arches: []string{"amd64"}}},
		{name: "windows/arm64", data: peFile(pe.IMAGE_FILE_MACHINE_ARM64), want: Binary{Format: FormatPE, GOOS: "windows", Arches: []string{"arm64"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got, err := Identify(tc.data); err != nil || !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Identify = %+v, %v, want %+v", got, err, tc.want)
			}
		})
	}
}

// TestIdentify_RefusesWhatTheReleaseDoesNotBuild covers every file that is
// not one of those binaries: a format none of the three, a file a reader
// refuses, and a readable binary for a machine the release does not build,
// which is named in the hex its format's headers spell it in.
func TestIdentify_RefusesWhatTheReleaseDoesNotBuild(t *testing.T) {
	t.Parallel()

	amd64, arm64 := machOFile(macho.CpuAmd64, 3), machOFile(macho.CpuArm64, 0)
	elf32 := newELF(elf.EM_386, "/lib/ld-linux.so.2")
	elf32.class = elf.ELFCLASS32
	badSignature := peFile(pe.IMAGE_FILE_MACHINE_AMD64)
	copy(badSignature[0x40:], "NE")
	for _, tc := range []struct {
		name string
		data []byte
		want string
	}{
		{name: "empty", data: nil, want: "is not an ELF, 64-bit Mach-O or PE file"},
		{name: "text", data: []byte("#!/bin/sh\necho hi\n"), want: "is not an ELF, 64-bit Mach-O or PE file"},
		{name: "a 32-bit Mach-O file", data: append(binary.LittleEndian.AppendUint32(nil, macho.Magic32), amd64[4:]...), want: "is not an ELF, 64-bit Mach-O or PE file"},
		{name: "a big-endian Mach-O file", data: append(binary.BigEndian.AppendUint32(nil, macho.Magic64), amd64[4:]...), want: "is not an ELF, 64-bit Mach-O or PE file"},
		{name: "a 32-bit ELF file", data: elf32.image(), want: "is not a 64-bit ELF file"},
		{name: "an ELF file for riscv64", data: newELF(elf.EM_RISCV, "/lib/ld-linux-riscv64-lp64d.so.1").image(), want: "is for ELF machine 0xf3, which the release does not build"},
		{name: "the Mach-O magic alone", data: amd64[:4], want: "is not a well-formed Mach-O file: "},
		{name: "a Mach-O file debug/macho refuses", data: amd64[:20], want: "is not a well-formed Mach-O file: "},
		{name: "a Mach-O file for 386", data: machOFile(macho.Cpu386, 3), want: "is for Mach-O cpu 0x7, which the release does not build"},
		{name: "the universal magic alone", data: goreleaserFat()[:4], want: "is not a well-formed universal Mach-O file: "},
		{name: "a universal file debug/macho refuses", data: goreleaserFat(), want: "is not a well-formed universal Mach-O file: file contains no images"},
		{name: "a universal file with one slice twice", data: goreleaserFat(arm64, arm64), want: "is not a well-formed universal Mach-O file: duplicate architecture"},
		{name: "a universal file with a ppc64 slice", data: goreleaserFat(arm64, machOFile(macho.CpuPpc64, 0)), want: "is for Mach-O cpu 0x1000012, which the release does not build"},
		{name: "a PE file debug/pe refuses", data: badSignature, want: "is not a well-formed PE file: invalid PE file signature"},
		{name: "an MZ stub too short for a DOS header", data: []byte("MZ"), want: "is not a well-formed PE file: "},
		{name: "a PE file for 386", data: peFile(pe.IMAGE_FILE_MACHINE_I386), want: "is for PE machine 0x14c, which the release does not build"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := Identify(tc.data)
			if err == nil || !strings.HasPrefix(err.Error(), tc.want) {
				t.Errorf("Identify = %+v, %v, want an error starting %q", got, err, tc.want)
			}
			if !reflect.DeepEqual(got, Binary{}) {
				t.Errorf("Identify refused the file and returned %+v, want nothing", got)
			}
		})
	}
}

// TestIdentify_TheRunningTestBinary identifies a binary no test built by
// hand: the test binary itself, which the go command linked for the host in
// the host's own format, so the readers meet a whole real file on each of the
// three operating systems CI runs on.
func TestIdentify_TheRunningTestBinary(t *testing.T) {
	t.Parallel()

	format, known := map[string]Format{"linux": FormatELF, "darwin": FormatMachO, "windows": FormatPE}[runtime.GOOS]
	if !known || (runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64") {
		t.Skipf("the release builds no %s/%s binary", runtime.GOOS, runtime.GOARCH)
	}
	path, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	data, err := os.ReadFile(path) //#nosec G304 -- the running test binary
	if err != nil {
		t.Fatalf("reading the test binary: %v", err)
	}
	want := Binary{Format: format, GOOS: runtime.GOOS, Arches: []string{runtime.GOARCH}}
	if got, identifyErr := Identify(data); identifyErr != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("Identify(%s) = %+v, %v, want %+v", path, got, identifyErr, want)
	}
}
