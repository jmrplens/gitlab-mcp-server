// host_test.go covers recording the machine a measurement came from: the two
// /proc parsers, and the sentence the documentation page prints.
package main

import (
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
	got := host.describe()
	for _, want := range []string{"AMD Ryzen 5 3550H", "8 logical CPUs", "61 GiB RAM", "linux/amd64", "kernel 6.1.0", "go1.27.1"} {
		if !strings.Contains(got, want) {
			t.Errorf("describe() = %q, missing %q", got, want)
		}
	}
}

// TestHostInfo_Describe_OmitsWhatItDoesNotKnow verifies a machine that would
// not answer produces a shorter sentence rather than one full of zeroes.
func TestHostInfo_Describe_OmitsWhatItDoesNotKnow(t *testing.T) {
	got := HostInfo{OS: "darwin", Arch: "arm64", CPUModel: "unknown", Kernel: "unknown", GoVersion: "go1.27.1"}.describe()
	if strings.Contains(got, "0 GiB") || strings.Contains(got, "0 logical") {
		t.Errorf("describe() = %q, want the unknown facts left out", got)
	}
	if strings.Contains(got, "kernel unknown") {
		t.Errorf("describe() = %q, want no kernel rather than an unknown one", got)
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
