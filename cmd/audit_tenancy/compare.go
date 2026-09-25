package main

import (
	"bytes"
	"debug/elf"
	"fmt"
	"io"
)

// lineTable is the one allocated section a pure move of a constant changes:
// Go's function and line table, which records the line every instruction came
// from, and a constant moved to another package moves the lines below it.
const lineTable = ".gopclntab"

// runCompare is -compare-binaries: two ELF files, and an exit code saying
// whether they hold the same code.
func runCompare(args []string, stdout, stderr io.Writer) int {
	if len(args) != 2 {
		fmt.Fprintf(stderr, "%s: -compare-binaries takes two binaries, got %d arguments\n", toolName, len(args))
		return 2
	}
	differences, err := compareBinaries(args[0], args[1])
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", toolName, err)
		return 1
	}
	for _, d := range differences {
		fmt.Fprintln(stdout, d)
	}
	if len(differences) > 0 {
		fmt.Fprintf(stdout, "%s: %s and %s differ in %d allocated sections besides %s\n", toolName, args[0], args[1], len(differences), lineTable)
		return 1
	}
	fmt.Fprintf(stdout, "%s: %s and %s are identical in every allocated section besides %s\n", toolName, args[0], args[1], lineTable)
	return 0
}

// compareBinaries lists how two ELF files differ in their allocated sections,
// the line table aside.
//
// The allocated sections are what the loader maps into the process: the
// instructions, the read-only data, the initialized data, the type and
// function metadata the runtime reads. The comparison is the proof a layer
// that only moves constants gives that it changed no code, and it was measured
// to be the right claim rather than an expectation: the compiler's assembly
// for such a package does change (it gains a relocation to the new import's
// init task), while every allocated section of the linked binary but the line
// table stays byte for byte the same. A section present in one binary and not
// the other is a difference, and so is a section of zero-filled memory whose
// size changed.
func compareBinaries(pathA, pathB string) ([]string, error) {
	a, err := allocatedSections(pathA)
	if err != nil {
		return nil, err
	}
	b, err := allocatedSections(pathB)
	if err != nil {
		return nil, err
	}
	var differences []string
	for _, name := range sortedKeys(a) {
		other, present := b[name]
		switch {
		case !present:
			differences = append(differences, fmt.Sprintf("%s: only in %s", name, pathA))
		case !bytes.Equal(a[name], other):
			differences = append(differences, fmt.Sprintf("%s: differs (%d bytes against %d)", name, len(a[name]), len(other)))
		}
	}
	for _, name := range sortedKeys(b) {
		if _, present := a[name]; !present {
			differences = append(differences, fmt.Sprintf("%s: only in %s", name, pathB))
		}
	}
	return differences, nil
}

// allocatedSections reads an ELF file's allocated sections but the line
// table, by name. A section of zero-filled memory has no bytes in the file,
// and is represented by its size.
func allocatedSections(path string) (map[string][]byte, error) {
	f, err := elf.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	defer f.Close()
	sections := map[string][]byte{}
	for _, s := range f.Sections {
		if s.Flags&elf.SHF_ALLOC == 0 || s.Name == lineTable {
			continue
		}
		if s.Type == elf.SHT_NOBITS {
			sections[s.Name] = fmt.Appendf(nil, "%d zero bytes", s.Size)
			continue
		}
		data, readErr := s.Data()
		if readErr != nil {
			return nil, fmt.Errorf("read section %s of %s: %w", s.Name, path, readErr)
		}
		sections[s.Name] = data
	}
	return sections, nil
}
