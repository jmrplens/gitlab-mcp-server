// host_test.go covers recording the machine a measurement came from: the two
// /proc parsers, and the sentence the documentation page prints.
package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestParseCPUModel_BothSpellings verifies the processor name is found under
// the key each architecture uses, and that an unusable file yields nothing
// rather than a wrong answer.
func TestParseCPUModel_BothSpellings(t *testing.T) {
	tests := []struct {
		name    string
		cpuinfo string
		want    string
	}{
		{
			name:    "x86",
			cpuinfo: "processor\t: 0\nvendor_id\t: AuthenticAMD\nmodel name\t: AMD Ryzen 5 3550H with Radeon Vega Mobile Gfx\n",
			want:    "AMD Ryzen 5 3550H with Radeon Vega Mobile Gfx",
		},
		{
			name:    "arm",
			cpuinfo: "processor\t: 0\nBogoMIPS\t: 108.00\nModel\t: Raspberry Pi 5\n",
			want:    "Raspberry Pi 5",
		},
		{name: "no model line", cpuinfo: "processor\t: 0\nflags\t: fpu vme\n", want: ""},
		{name: "empty", cpuinfo: "", want: ""},
		{name: "blank value", cpuinfo: "model name\t:   \n", want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseCPUModel(tc.cpuinfo); got != tc.want {
				t.Errorf("parseCPUModel = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestParseMemTotalKiB_ReadsTheFirstField verifies installed memory is read
// from MemTotal and not from the MemFree line just below it.
func TestParseMemTotalKiB_ReadsTheFirstField(t *testing.T) {
	meminfo := "MemTotal:       63729784 kB\nMemFree:         1104924 kB\nMemAvailable:   25729000 kB\n"
	if got := parseMemTotalKiB(meminfo); got != 63729784 {
		t.Errorf("parseMemTotalKiB = %v, want 63729784", got)
	}
	if got := parseMemTotalKiB("MemFree: 1 kB\n"); got != 0 {
		t.Errorf("parseMemTotalKiB with no MemTotal = %v, want 0", got)
	}
}

// TestHostInfo_Describe_NamesEverythingAReaderNeeds verifies the sentence the
// page prints carries the processor, the platform and the toolchain, because
// a resident-set figure with none of those is not reproducible.
func TestHostInfo_Describe_NamesEverythingAReaderNeeds(t *testing.T) {
	host := HostInfo{
		OS: "linux", Arch: "amd64", CPUModel: "AMD Ryzen 5 3550H",
		CPUs: 8, MemTotalGiB: 61, Kernel: "6.1.0-52-amd64", GoVersion: "go1.27.1",
	}
	got := host.describe(englishLabels())
	for _, want := range []string{"AMD Ryzen 5 3550H", "8 logical CPUs", "61 GiB RAM", "linux/amd64", "kernel 6.1.0", "go1.27.1"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(got, want) {
				t.Errorf("describe() = %q, missing %q", got, want)
			}
		})
	}
}

// TestHostInfo_Describe_OmitsWhatItDoesNotKnow verifies a machine that would
// not answer produces a shorter sentence rather than one full of zeroes.
func TestHostInfo_Describe_OmitsWhatItDoesNotKnow(t *testing.T) {
	got := HostInfo{OS: "darwin", Arch: "arm64", CPUModel: "unknown", Kernel: "unknown", GoVersion: "go1.27.1"}.
		describe(englishLabels())
	if strings.Contains(got, "0 GiB") || strings.Contains(got, "0 logical") {
		t.Errorf("describe() = %q, want the unknown facts left out", got)
	}
	if strings.Contains(got, "kernel unknown") {
		t.Errorf("describe() = %q, want no kernel rather than an unknown one", got)
	}
}

// TestHostInfo_Describe_FollowsTheLanguage verifies the sentence is written in
// the language of the page it is going on, so the Spanish page does not carry
// an English fragment inside a translated sentence.
func TestHostInfo_Describe_FollowsTheLanguage(t *testing.T) {
	host := HostInfo{
		OS: "linux", Arch: "amd64", CPUModel: "Test CPU",
		CPUs: 8, MemTotalGiB: 61, Kernel: "6.1.0", GoVersion: "go1.27.1",
	}
	spanish := host.describe(spanishLabels())
	if strings.Contains(spanish, "logical CPUs") || strings.Contains(spanish, "GiB RAM") {
		t.Errorf("the Spanish description carries English words: %q", spanish)
	}
	if !strings.Contains(spanish, spanishLabels().HostCPUs) {
		t.Errorf("the Spanish description does not use its own wording: %q", spanish)
	}
}

// TestParseCPUModel_SkipsLinesWithoutAKey verifies a line with no separator
// is passed over rather than read as a model with an empty name.
func TestParseCPUModel_SkipsLinesWithoutAKey(t *testing.T) {
	if got := parseCPUModel("bogus line\nmodel name\t: Real CPU\n"); got != "Real CPU" {
		t.Errorf("parseCPUModel = %q, want the model after the bogus line", got)
	}
}

// fakeSysctl puts a sysctl on the PATH that answers the two questions the
// macOS branches ask, with the memory size the test wants, and returns the
// directory so PATH can be narrowed to it.
func fakeSysctl(t *testing.T, memsize string) string {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\ncase \"$2\" in\n  machdep.cpu.brand_string) echo 'Fake M9';;\n  hw.memsize) echo '" + memsize + "';;\nesac\n"
	//#nosec G703 -- both halves of the path are this test's own: a t.TempDir and a literal
	if err := os.WriteFile(filepath.Join(dir, "sysctl"), []byte(script), 0o700); err != nil { //#nosec G306 -- a script the test must execute
		t.Fatalf("write the fake sysctl: %v", err)
	}
	return dir
}

// TestHostFacts_OtherPlatforms walks the branches only another operating
// system takes, by declaring that system on this one: macOS asks sysctl,
// which is stood in for here, and every other platform admits it knows
// nothing. The uname fallback is reached by taking uname off the PATH.
func TestHostFacts_OtherPlatforms(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a shell script cannot stand in for sysctl on Windows")
	}
	previous := runtimeGOOS
	t.Cleanup(func() { runtimeGOOS = previous })

	t.Run("macOS with a sysctl that answers", func(t *testing.T) {
		runtimeGOOS = "darwin"
		t.Setenv("PATH", fakeSysctl(t, "17179869184"))
		if got := cpuModel(); got != "Fake M9" {
			t.Errorf("cpuModel = %q, want what sysctl said", got)
		}
		if got := totalMemoryGiB(); got != 16 {
			t.Errorf("totalMemoryGiB = %v, want 16 from the 17179869184 bytes sysctl reported", got)
		}
		if got := availableMemoryMiB(); got != 0 {
			t.Errorf("availableMemoryMiB = %v on a platform with no MemAvailable, want 0", got)
		}
		if got := kernelRelease(); got != "unknown" {
			t.Errorf("kernelRelease = %q with no uname on the PATH, want unknown", got)
		}
	})

	t.Run("macOS with a sysctl that answers nonsense", func(t *testing.T) {
		runtimeGOOS = "darwin"
		t.Setenv("PATH", fakeSysctl(t, "lots"))
		if got := totalMemoryGiB(); got != 0 {
			t.Errorf("totalMemoryGiB = %v from an unparseable sysctl, want 0", got)
		}
	})

	t.Run("a platform with no source at all", func(t *testing.T) {
		runtimeGOOS = "plan9"
		t.Setenv("PATH", t.TempDir())
		if got := cpuModel(); got != "unknown" {
			t.Errorf("cpuModel = %q, want unknown", got)
		}
		if got := totalMemoryGiB(); got != 0 {
			t.Errorf("totalMemoryGiB = %v, want 0", got)
		}
	})

	t.Run("linux reads available memory", func(t *testing.T) {
		runtimeGOOS = "linux"
		got := availableMemoryMiB()
		if runtime.GOOS == "linux" && got <= 0 {
			t.Errorf("availableMemoryMiB = %v on Linux, want what /proc/meminfo says", got)
		}
	})
}

// TestParseMeminfoKiB_ReadsTheNamedField verifies each field is read by its
// own name, since MemTotal and MemAvailable sit lines apart in the same file
// and mean different things to the budget.
func TestParseMeminfoKiB_ReadsTheNamedField(t *testing.T) {
	meminfo := "MemTotal:       63729784 kB\nMemFree:         1104924 kB\nMemAvailable:   25729000 kB\nBroken: x kB\n"
	cases := []struct {
		field string
		want  float64
	}{
		{field: "MemTotal:", want: 63729784},
		{field: "MemAvailable:", want: 25729000},
		{field: "Broken:", want: 0},
		{field: "Missing:", want: 0},
	}
	for _, tc := range cases {
		t.Run(tc.field, func(t *testing.T) {
			if got := parseMeminfoKiB(meminfo, tc.field); got != tc.want {
				t.Errorf("parseMeminfoKiB(%s) = %v, want %v", tc.field, got, tc.want)
			}
		})
	}
}

// TestHostInfo_RealMachine_FillsThePlatformFields verifies the collector
// reports what the Go runtime always knows, whatever the operating system will
// or will not say about the rest.
func TestHostInfo_RealMachine_FillsThePlatformFields(t *testing.T) {
	host := hostInfo()
	if host.OS == "" || host.Arch == "" || host.GoVersion == "" || host.CPUs < 1 {
		t.Errorf("hostInfo did not fill the runtime-known fields: %+v", host)
	}
}
