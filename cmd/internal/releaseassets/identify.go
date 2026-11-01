// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package releaseassets

import (
	"bytes"
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"encoding/binary"
	"errors"
	"fmt"
)

// Format is the executable format of a release binary.
type Format string

// The formats the release publishes its binaries in.
const (
	// FormatELF is a 64-bit little-endian ELF file, the linux binaries.
	FormatELF Format = "ELF"
	// FormatMachO is a thin 64-bit little-endian Mach-O file, one darwin
	// architecture.
	FormatMachO Format = "Mach-O"
	// FormatUniversal is a universal (fat) Mach-O file, the darwin
	// architectures joined.
	FormatUniversal Format = "universal Mach-O"
	// FormatPE is a PE file, the windows binaries.
	FormatPE Format = "PE"
)

// Binary is what identifies a release binary: its format, the operating
// system it runs on, and the architecture of each image it holds, in the
// file's own order. Every format holds one image except the universal one.
type Binary struct {
	Format Format
	GOOS   string
	Arches []string
}

// The architecture each format's header names for the two the release
// builds.
var (
	elfArches = map[elf.Machine]string{
		elf.EM_X86_64:  "amd64",
		elf.EM_AARCH64: "arm64",
	}
	machOArches = map[macho.Cpu]string{
		macho.CpuAmd64: "amd64",
		macho.CpuArm64: "arm64",
	}
	peArches = map[uint16]string{
		pe.IMAGE_FILE_MACHINE_AMD64: "amd64",
		pe.IMAGE_FILE_MACHINE_ARM64: "arm64",
	}
)

// The first bytes of a universal Mach-O file, whose header is big-endian, and
// of a 64-bit little-endian thin one.
var (
	universalMagic = binary.BigEndian.AppendUint32(nil, macho.MagicFat)
	machOMagic     = binary.LittleEndian.AppendUint32(nil, macho.Magic64)
)

// Identify reads a whole release binary and names its target.
//
// The format is chosen by the file's first bytes, the ELF magic, the
// universal Mach-O magic, the 64-bit little-endian Mach-O magic or the MZ of
// a PE file's DOS stub, and the file is then read with debug/elf, debug/macho
// or debug/pe, which refuse one whose headers do not hold together. A
// readable binary for another machine than amd64 or arm64 is refused with the
// machine named, since the release builds no other.
func Identify(data []byte) (Binary, error) {
	switch {
	case bytes.HasPrefix(data, []byte(elf.ELFMAG)):
		return identifyELF(data)
	case bytes.HasPrefix(data, universalMagic):
		return identifyUniversal(data)
	case bytes.HasPrefix(data, machOMagic):
		return identifyMachO(data)
	case bytes.HasPrefix(data, []byte("MZ")):
		return identifyPE(data)
	}
	return Binary{}, errors.New("is not an ELF, 64-bit Mach-O or PE file")
}

// identifyELF identifies a linux binary.
func identifyELF(data []byte) (Binary, error) {
	img, err := ReadELF(data)
	if err != nil {
		return Binary{}, err
	}
	arch, ok := elfArches[img.Machine]
	if !ok {
		return Binary{}, fmt.Errorf("is for ELF machine %#x, which the release does not build", uint16(img.Machine))
	}
	return Binary{Format: FormatELF, GOOS: "linux", Arches: []string{arch}}, nil
}

// identifyMachO identifies a thin darwin binary.
func identifyMachO(data []byte) (Binary, error) {
	f, err := macho.NewFile(bytes.NewReader(data))
	if err != nil {
		return Binary{}, fmt.Errorf("is not a well-formed Mach-O file: %w", err)
	}
	arch, err := machOArch(f.Cpu)
	if err != nil {
		return Binary{}, err
	}
	return Binary{Format: FormatMachO, GOOS: "darwin", Arches: []string{arch}}, nil
}

// identifyUniversal identifies a universal darwin binary, every slice of
// which debug/macho reads as a Mach-O file of its own.
func identifyUniversal(data []byte) (Binary, error) {
	ff, err := macho.NewFatFile(bytes.NewReader(data))
	if err != nil {
		return Binary{}, fmt.Errorf("is not a well-formed universal Mach-O file: %w", err)
	}
	arches := make([]string, 0, len(ff.Arches))
	for _, slice := range ff.Arches {
		arch, archErr := machOArch(slice.Cpu)
		if archErr != nil {
			return Binary{}, archErr
		}
		arches = append(arches, arch)
	}
	return Binary{Format: FormatUniversal, GOOS: "darwin", Arches: arches}, nil
}

// machOArch is the architecture a Mach-O cpu type names.
func machOArch(cpu macho.Cpu) (string, error) {
	arch, ok := machOArches[cpu]
	if !ok {
		return "", fmt.Errorf("is for Mach-O cpu %#x, which the release does not build", uint32(cpu))
	}
	return arch, nil
}

// identifyPE identifies a windows binary.
func identifyPE(data []byte) (Binary, error) {
	f, err := pe.NewFile(bytes.NewReader(data))
	if err != nil {
		return Binary{}, fmt.Errorf("is not a well-formed PE file: %w", err)
	}
	arch, ok := peArches[f.Machine]
	if !ok {
		return Binary{}, fmt.Errorf("is for PE machine %#x, which the release does not build", f.Machine)
	}
	return Binary{Format: FormatPE, GOOS: "windows", Arches: []string{arch}}, nil
}
