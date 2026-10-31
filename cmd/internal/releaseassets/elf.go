// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package releaseassets

import (
	"bytes"
	"debug/elf"
	"errors"
	"fmt"
)

// elf64HeaderSize is the size of a 64-bit ELF file header, which a file has
// to hold before anything in it can be read.
const elf64HeaderSize = 64

// ELFImage is what a release holds a Linux binary to: the machine it was
// built for and the loader the kernel starts it through.
type ELFImage struct {
	// Machine is the header's e_machine.
	Machine elf.Machine
	// Interpreter is the path the PT_INTERP program header names, up to its
	// NUL, and empty for an image without one, which is what a statically
	// linked binary is.
	Interpreter string
}

// ReadELF reads the machine and the interpreter of a 64-bit little-endian
// ELF image, the only kind the release builds.
//
// The identifier is judged first, as the kernel judges it: a file shorter
// than a 64-bit header or without the ELF magic is not an ELF file, and one
// of another class or byte order is refused by name. debug/elf then reads the
// header and the program headers, refusing a table that runs past the end of
// the file among the rest. An interpreter that runs past the end is refused,
// and so is an image naming two, which the kernel refuses to start.
func ReadELF(data []byte) (ELFImage, error) {
	if len(data) < elf64HeaderSize || !bytes.HasPrefix(data, []byte(elf.ELFMAG)) {
		return ELFImage{}, errors.New("is not an ELF file")
	}
	if elf.Class(data[elf.EI_CLASS]) != elf.ELFCLASS64 {
		return ELFImage{}, errors.New("is not a 64-bit ELF file")
	}
	if elf.Data(data[elf.EI_DATA]) != elf.ELFDATA2LSB {
		return ELFImage{}, errors.New("is not a little-endian ELF file")
	}
	f, err := elf.NewFile(bytes.NewReader(data))
	if err != nil {
		return ELFImage{}, fmt.Errorf("is not a well-formed ELF file: %w", err)
	}

	var interpreters []string
	for _, prog := range f.Progs {
		if prog.Type != elf.PT_INTERP {
			continue
		}
		// debug/elf refuses an offset or a size that does not fit in an
		// int64, so the sum of the two cannot overflow.
		end := prog.Off + prog.Filesz
		if end > uint64(len(data)) {
			return ELFImage{}, errors.New("its PT_INTERP runs past the end of the file")
		}
		path, _, _ := bytes.Cut(data[prog.Off:end], []byte{0})
		interpreters = append(interpreters, string(path))
	}
	if len(interpreters) > 1 {
		return ELFImage{}, fmt.Errorf("has %d PT_INTERP headers, which the kernel refuses to start", len(interpreters))
	}
	img := ELFImage{Machine: f.Machine}
	if len(interpreters) == 1 {
		img.Interpreter = interpreters[0]
	}
	return img, nil
}
