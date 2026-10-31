// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package releaseassets

import (
	"bytes"
	"debug/elf"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"runtime"
	"strings"
	"testing"
)

// The loaders the release's Linux binaries name, the glibc ones the
// per-target overrides in .goreleaser.yml pass with -I.
const (
	glibcAMD64 = "/lib64/ld-linux-x86-64.so.2"
	glibcARM64 = "/lib/ld-linux-aarch64.so.1"
)

// elfSpec describes a minimal ELF image the way scripts/check_elf_interp_test.py
// describes one: a 64-byte file header, a program header table right after it
// holding leadingLoads PT_LOAD entries and then one PT_INTERP per entry of
// interpreters, and the interpreter strings after the table, NUL-terminated as
// the linker writes them. Every field a case does not set is what a correct
// build writes; e_version is one the Python image leaves at zero, and the one
// debug/elf holds to the identifier's, as a linker writes it.
type elfSpec struct {
	machine        elf.Machine
	interpreters   []string
	class          elf.Class
	encoding       elf.Data
	version        uint32
	leadingLoads   int
	phentsize      int
	truncateInterp bool
}

// newELF is the image a correct build of machine writes, naming
// interpreters.
func newELF(machine elf.Machine, interpreters ...string) elfSpec {
	return elfSpec{
		machine:      machine,
		interpreters: interpreters,
		class:        elf.ELFCLASS64,
		encoding:     elf.ELFDATA2LSB,
		version:      uint32(elf.EV_CURRENT),
		leadingLoads: 1,
		phentsize:    56,
	}
}

// image writes the bytes. truncateInterp declares each PT_INTERP 4096 bytes
// longer than the string it points at, so it runs past the end of the file.
func (s elfSpec) image() []byte {
	phnum := s.leadingLoads + len(s.interpreters)
	header := make([]byte, 64)
	copy(header, elf.ELFMAG)
	header[elf.EI_CLASS] = byte(s.class)
	header[elf.EI_DATA] = byte(s.encoding)
	header[elf.EI_VERSION] = byte(elf.EV_CURRENT)
	binary.LittleEndian.PutUint16(header[18:], uint16(s.machine))
	binary.LittleEndian.PutUint32(header[20:], s.version)
	binary.LittleEndian.PutUint64(header[32:], 64)
	binary.LittleEndian.PutUint16(header[54:], uint16(s.phentsize)) //#nosec G115 -- a fixture's entry size, under a hundred bytes
	binary.LittleEndian.PutUint16(header[56:], uint16(phnum))       //#nosec G115 -- a fixture's entry count, a handful

	table := make([]byte, s.phentsize*phnum)
	var text []byte
	textAt := len(header) + len(table)
	for i := range phnum {
		entry := table[i*s.phentsize:]
		if i < s.leadingLoads {
			binary.LittleEndian.PutUint32(entry, uint32(elf.PT_LOAD))
			continue
		}
		interp := append([]byte(s.interpreters[i-s.leadingLoads]), 0)
		size := len(interp)
		if s.truncateInterp {
			size += 4096
		}
		binary.LittleEndian.PutUint32(entry, uint32(elf.PT_INTERP))
		binary.LittleEndian.PutUint64(entry[8:], uint64(textAt+len(text))) //#nosec G115 -- an offset inside a fixture of a few hundred bytes
		binary.LittleEndian.PutUint64(entry[32:], uint64(size))            //#nosec G115 -- a fixture's string length
		text = append(text, interp...)
	}
	return append(append(header, table...), text...)
}

// TestReadELF_ReadsMachineAndInterpreter covers the images a release binary
// can be, and the bounds a well-formed one can sit on: a table or an
// interpreter ending at the file's last byte, and a file that is its header
// alone.
func TestReadELF_ReadsMachineAndInterpreter(t *testing.T) {
	t.Parallel()

	interpFirst := newELF(elf.EM_AARCH64, glibcARM64)
	interpFirst.leadingLoads = 0
	wide := newELF(elf.EM_X86_64, glibcAMD64)
	wide.phentsize = 64
	headerAlone := newELF(elf.EM_X86_64)
	headerAlone.leadingLoads = 0
	for _, tc := range []struct {
		name string
		data []byte
		size int
		want ELFImage
	}{
		{name: "amd64 glibc", data: newELF(elf.EM_X86_64, glibcAMD64).image(), want: ELFImage{Machine: elf.EM_X86_64, Interpreter: glibcAMD64}},
		{name: "arm64 glibc", data: newELF(elf.EM_AARCH64, glibcARM64).image(), want: ELFImage{Machine: elf.EM_AARCH64, Interpreter: glibcARM64}},
		{name: "interpreter first in the table", data: interpFirst.image(), want: ELFImage{Machine: elf.EM_AARCH64, Interpreter: glibcARM64}},
		{name: "static: no PT_INTERP", data: newELF(elf.EM_X86_64).image(), size: 64 + 56, want: ELFImage{Machine: elf.EM_X86_64}},
		{name: "wider entries than the minimum", data: wide.image(), want: ELFImage{Machine: elf.EM_X86_64, Interpreter: glibcAMD64}},
		{name: "the header alone", data: headerAlone.image(), size: 64, want: ELFImage{Machine: elf.EM_X86_64}},
		{name: "another machine", data: newELF(elf.EM_RISCV, "/lib/ld-linux-riscv64-lp64d.so.1").image(), want: ELFImage{Machine: elf.EM_RISCV, Interpreter: "/lib/ld-linux-riscv64-lp64d.so.1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if tc.size != 0 && len(tc.data) != tc.size {
				t.Fatalf("the fixture is %d bytes, want %d", len(tc.data), tc.size)
			}
			got, err := ReadELF(tc.data)
			if err != nil || got != tc.want {
				t.Errorf("ReadELF = %+v, %v, want %+v", got, err, tc.want)
			}
		})
	}
}

// TestReadELF_AnInterpreterEndingAtTheFileEndIsRead covers the bound on the
// interpreter: past the end is refused, at the end is whole.
func TestReadELF_AnInterpreterEndingAtTheFileEndIsRead(t *testing.T) {
	t.Parallel()

	data := newELF(elf.EM_AARCH64, glibcARM64).image()
	if !bytes.HasSuffix(data, append([]byte(glibcARM64), 0)) {
		t.Fatalf("the fixture does not end with its interpreter")
	}
	if got, err := ReadELF(data); err != nil || got.Interpreter != glibcARM64 {
		t.Errorf("ReadELF = %+v, %v, want the interpreter %s", got, err, glibcARM64)
	}
}

// TestReadELF_RefusesWhatIsNotAReadableELF64LE covers every image the release
// cannot have built: not ELF at all, the wrong class or byte order, a header
// debug/elf refuses, a table or an interpreter running past the end, and two
// interpreters, which the kernel refuses to start.
func TestReadELF_RefusesWhatIsNotAReadableELF64LE(t *testing.T) {
	t.Parallel()

	good := newELF(elf.EM_X86_64, glibcAMD64)
	class32 := good
	class32.class = elf.ELFCLASS32
	bigEndian := good
	bigEndian.encoding = elf.ELFDATA2MSB
	shortEntries := good
	shortEntries.phentsize = 55
	truncated := good
	truncated.truncateInterp = true
	unversioned := good
	unversioned.version = 0
	for _, tc := range []struct {
		name string
		data []byte
		want string
		is   error
	}{
		{name: "empty", data: nil, want: "is not an ELF file"},
		{name: "short of a header", data: append([]byte(elf.ELFMAG), make([]byte, 59)...), want: "is not an ELF file"},
		{name: "wrong magic", data: append([]byte("MZ"), good.image()[2:]...), want: "is not an ELF file"},
		{name: "32-bit", data: class32.image(), want: "is not a 64-bit ELF file"},
		{name: "big-endian", data: bigEndian.image(), want: "is not a little-endian ELF file"},
		{name: "short entries", data: shortEntries.image(), want: "is not a well-formed ELF file: invalid ELF phentsize"},
		{name: "table past the end", data: good.image()[:100], want: "is not a well-formed ELF file", is: io.EOF},
		{name: "e_version other than the identifier's", data: unversioned.image(), want: "is not a well-formed ELF file: mismatched ELF version"},
		{name: "interpreter past the end", data: truncated.image(), want: "its PT_INTERP runs past the end of the file"},
		{name: "two interpreters", data: newELF(elf.EM_X86_64, glibcAMD64, glibcAMD64).image(), want: "has 2 PT_INTERP headers, which the kernel refuses to start"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := ReadELF(tc.data)
			if err == nil || !strings.HasPrefix(err.Error(), tc.want) {
				t.Errorf("ReadELF = %+v, %v, want an error starting %q", got, err, tc.want)
			}
			if tc.is != nil && !errors.Is(err, tc.is) {
				t.Errorf("ReadELF = %v, want it to wrap %v", err, tc.is)
			}
			if got != (ELFImage{}) {
				t.Errorf("ReadELF refused the image and returned %+v, want nothing", got)
			}
		})
	}
}

// TestReadELF_ReadsARealBinary reads the host's own shell, which on a glibc or
// musl Linux host is a dynamically linked ELF file, so the reader has to find
// its loader where the kernel does, which an offset off by one would not.
func TestReadELF_ReadsARealBinary(t *testing.T) {
	t.Parallel()

	want, ok := map[string]elf.Machine{"amd64": elf.EM_X86_64, "arm64": elf.EM_AARCH64}[runtime.GOARCH]
	if runtime.GOOS != "linux" || !ok {
		t.Skipf("the host's binaries are ELF files of a machine the release builds only on linux/amd64 and linux/arm64, not %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	data, err := os.ReadFile("/bin/sh")
	if err != nil {
		t.Skipf("no shell to read: %v", err)
	}
	got, err := ReadELF(data)
	if err != nil {
		t.Fatalf("ReadELF(/bin/sh): %v", err)
	}
	if got.Machine != want {
		t.Errorf("ReadELF(/bin/sh) machine = %v, want %v", got.Machine, want)
	}
	if got.Interpreter == "" {
		t.Skip("this host's shell is statically linked")
	}
	if !strings.HasPrefix(got.Interpreter, "/") {
		t.Errorf("ReadELF(/bin/sh) interpreter = %q, want an absolute path", got.Interpreter)
	}
}
