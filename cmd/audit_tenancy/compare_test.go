package main

import (
	"debug/elf"
	"encoding/binary"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// testBinary copies the running test binary, the one ELF file every Linux run
// of this package is sure to have, and returns the copy's path with its bytes
// and its sections. Anywhere the binary is not a 64-bit little-endian ELF the
// case is skipped: the comparison is layer evidence taken on Linux, and the
// patches below write that layout.
func testBinary(t *testing.T) (string, []byte, *elf.File) {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Skipf("no path to the test binary: %v", err)
	}
	f, err := elf.Open(self)
	if err != nil {
		t.Skipf("the test binary is not ELF here: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })
	if f.Class != elf.ELFCLASS64 || f.ByteOrder != binary.LittleEndian {
		t.Skip("the patches below write a 64-bit little-endian ELF")
	}
	data, err := os.ReadFile(self)
	if err != nil {
		t.Fatalf("read the test binary: %v", err)
	}
	return writeCopy(t, data), data, f
}

// writeCopy writes data to a new file and returns its path.
func writeCopy(t *testing.T, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "binary")
	//#nosec G703 -- both halves of the path are this test's own: a t.TempDir and a literal
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write copy: %v", err)
	}
	return path
}

// section returns the named section and its index in the header table.
func section(t *testing.T, f *elf.File, name string) (*elf.Section, int) {
	t.Helper()
	for i, s := range f.Sections {
		if s.Name == name {
			return s, i
		}
	}
	t.Skipf("the test binary has no %s section", name)
	return nil, 0
}

// sectionHeader is the file offset of the i'th section header.
func sectionHeader(data []byte, i int) int {
	shoff := binary.LittleEndian.Uint64(data[0x28:])
	shentsize := binary.LittleEndian.Uint16(data[0x3A:])
	//#nosec G115 -- an offset inside the test binary, which is far smaller than the conversion's range
	return int(shoff) + i*int(shentsize)
}

// patched is a copy of data with fn applied to it.
func patched(t *testing.T, data []byte, fn func([]byte)) string {
	t.Helper()
	out := slices.Clone(data)
	fn(out)
	return writeCopy(t, out)
}

// TestCompareBinaries_ACopy_IsIdentical, and runCompare says so and exits 0.
func TestCompareBinaries_ACopy_IsIdentical(t *testing.T) {
	path, data, _ := testBinary(t)
	other := writeCopy(t, data)
	var stdout, stderr strings.Builder
	if code := runCompare([]string{path, other}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit = %d, stdout = %q, stderr = %q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "are identical in every allocated section besides .gopclntab") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

// TestCompareBinaries_AByteOfCode_Differs: one flipped byte inside .text is
// a difference, and runCompare names the section and exits 1.
func TestCompareBinaries_AByteOfCode_Differs(t *testing.T) {
	path, data, f := testBinary(t)
	text, _ := section(t, f, ".text")
	other := patched(t, data, func(b []byte) { b[text.Offset+16] ^= 0xFF })
	var stdout, stderr strings.Builder
	if code := runCompare([]string{path, other}, &stdout, &stderr); code != 1 {
		t.Fatalf("exit = %d, want 1; stdout = %q", code, stdout.String())
	}
	if !strings.HasPrefix(stdout.String(), ".text: differs (") || !strings.Contains(stdout.String(), "differ in 1 allocated sections besides .gopclntab") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

// TestCompareBinaries_ALineTableByte_IsNotADifference: .gopclntab is the one
// allocated section the comparison sets aside, because a moved line changes
// it; it is not the only section a move can change, and every other one is
// compared.
func TestCompareBinaries_ALineTableByte_IsNotADifference(t *testing.T) {
	path, data, f := testBinary(t)
	table, _ := section(t, f, ".gopclntab")
	other := patched(t, data, func(b []byte) { b[table.Offset+table.Size/2] ^= 0xFF })
	differences, err := compareBinaries(path, other)
	if err != nil || len(differences) != 0 {
		t.Fatalf("differences = %v, err = %v, want none", differences, err)
	}
}

// TestCompareBinaries_ZeroFilledMemoryOfAnotherSize_Differs: a section with no
// bytes in the file is compared by its size.
func TestCompareBinaries_ZeroFilledMemoryOfAnotherSize_Differs(t *testing.T) {
	path, data, f := testBinary(t)
	_, index := section(t, f, ".bss")
	other := patched(t, data, func(b []byte) {
		size := sectionHeader(b, index) + 0x20
		binary.LittleEndian.PutUint64(b[size:], binary.LittleEndian.Uint64(b[size:])+8)
	})
	differences, err := compareBinaries(path, other)
	if err != nil || !slices.ContainsFunc(differences, func(d string) bool { return strings.HasPrefix(d, ".bss: differs") }) {
		t.Fatalf("differences = %v, err = %v, want .bss", differences, err)
	}
}

// TestCompareBinaries_ASectionInOnlyOne_Differs both ways: a section renamed
// in the copy is one only the original has, and one only the copy has.
func TestCompareBinaries_ASectionInOnlyOne_Differs(t *testing.T) {
	path, data, f := testBinary(t)
	names, _ := section(t, f, ".shstrtab")
	_, index := section(t, f, ".noptrdata")
	other := patched(t, data, func(b []byte) {
		name := binary.LittleEndian.Uint32(b[sectionHeader(b, index):])
		b[names.Offset+uint64(name)+uint64(len(".noptrdat"))] = 'X'
	})
	differences, err := compareBinaries(path, other)
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	for name, want := range map[string]string{"original": ".noptrdata: only in " + path, "copy": ".noptrdatX: only in " + other} {
		t.Run(name, func(t *testing.T) {
			if !slices.Contains(differences, want) {
				t.Fatalf("differences = %v, want %q among them", differences, want)
			}
		})
	}
}

// TestCompareBinaries_WhatCannotBeRead_IsAnError: a file that is not ELF, on
// either side, and a section whose bytes lie past the end of the file.
func TestCompareBinaries_WhatCannotBeRead_IsAnError(t *testing.T) {
	path, data, f := testBinary(t)
	notELF := writeCopy(t, []byte("not an ELF file"))
	_, index := section(t, f, ".text")
	truncated := patched(t, data, func(b []byte) {
		binary.LittleEndian.PutUint64(b[sectionHeader(b, index)+0x18:], uint64(len(b))*2)
	})
	for name, pair := range map[string][2]string{
		"first not ELF":    {notELF, path},
		"second not ELF":   {path, notELF},
		"section past EOF": {path, truncated},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := compareBinaries(pair[0], pair[1]); err == nil {
				t.Fatal("compare succeeded, want an error")
			}
			var stdout, stderr strings.Builder
			if code := runCompare(pair[:], &stdout, &stderr); code != 1 || !strings.HasPrefix(stderr.String(), toolName+": ") {
				t.Fatalf("exit = %d, stderr = %q, want 1 and the error", code, stderr.String())
			}
		})
	}
}
