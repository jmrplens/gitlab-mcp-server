// figures.go turns one measurement record into the four figures the
// documentation publishes, and carries the strings each language needs.
//
// The figures were chosen to answer the three questions the record exists for.
// How much memory does a deployment need, and does the answer change with the
// number of clients: the first two. What does a client wait for, and where
// does that wait actually live: the third. What does a request cost once
// everything is warm: the fourth.
package main

import (
	"fmt"
	"strings"
)

// figure is one chart, renderable in either palette.
type figure struct {
	Name   string
	Render func(palette) string
}

// labels are the words one language's figures and tables are written in.
//
// The Spanish site page carries no English, so the charts on it cannot either:
// a rendered SVG is content, not chrome. Configuration values (dynamic, meta,
// individual) and protocol method names (tools/list) stay as they are in both,
// because those are literals a reader types, not prose.
type labels struct {
	Code string

	MemoryTitle     string
	MemorySubtitle  string
	MemoryY         string
	MemoryStdioOne  string
	MemoryHTTPOne   string
	MemoryHTTPAll   string
	MemoryThreshold string

	RampTitle    string
	RampSubtitle string
	RampX        string

	StartupTitle    string
	StartupSubtitle string
	StartupY        string
	StartupReady    string
	StartupCold     string
	StartupWarm     string

	LatencyTitle    string
	LatencySubtitle string

	MeasuredOn string
	// HostCPUs, HostRAM and HostKernel are the words the machine's own
	// description is written with. The sentence naming the host is prose, so
	// it is translated like the rest of it rather than left in English inside
	// a Spanish page.
	HostCPUs     string
	HostRAM      string
	HostKernel   string
	SummaryHead  []string
	StartupHead  []string
	LatencyHead  []string
	SurfaceNote  string
	FigureAlt    map[string]string
	TableCaption map[string]string

	// CallDetail translates the descriptions the record stores beside a
	// method. The record itself stays English, because it is a project
	// artifact and because the tool names in that same column
	// (gitlab_find_action) are not words in any language; a description with
	// no entry here is printed as recorded.
	CallDetail map[string]string
}

// callDetail renders the description recorded beside a method, in this page's
// language.
func (l labels) callDetail(recorded string) string {
	if translated, ok := l.CallDetail[recorded]; ok {
		return translated
	}
	return recorded
}

// englishLabels is the wording of the English page and of the Markdown
// documentation.
//
//nolint:dupl // one bundle per language, identical in shape by definition
func englishLabels() labels {
	return labels{
		Code:            "en",
		MemoryTitle:     "Resident memory by tool surface",
		MemorySubtitle:  "stdio holds one catalog per process; HTTP holds one per credential in the pool",
		MemoryY:         "Resident set (MiB)",
		MemoryStdioOne:  "stdio, one process",
		MemoryHTTPOne:   "HTTP, one credential",
		MemoryHTTPAll:   "HTTP, %d credentials",
		MemoryThreshold: "512 MiB",
		RampTitle:       "Resident memory as credentials arrive",
		RampSubtitle:    "HTTP mode: every distinct token builds its own catalog in the pool",
		RampX:           "live credentials (pool entries)",
		StartupTitle:    "What a client waits for",
		StartupSubtitle: "HTTP mode, log scale: the catalog is built on the first tools/list, not at startup",
		StartupY:        "milliseconds (log scale)",
		StartupReady:    "process ready",
		StartupCold:     "first tools/list (cold)",
		StartupWarm:     "tools/list once warm (p50)",
		LatencyTitle:    "Request latency by method",
		LatencySubtitle: "solid bar p50, faint extension p99, log scale",
		MeasuredOn:      "Measured on %s, build %s, %s. %d rounds per method, resident set sampled every %d ms.",
		HostCPUs:        "logical CPUs",
		HostRAM:         "GiB RAM",
		HostKernel:      "kernel",
		SummaryHead: []string{
			"Scenario", "Clients", "Idle", "One client", "All clients", "Per extra client",
			"Peak", "Goroutines", "CPU, % of one core",
		},
		StartupHead: []string{
			"Scenario", "Process ready", "First tools/list", "Warm tools/list (p50)", "tools/list payload",
		},
		LatencyHead: []string{"Scenario", "Method", "Call", "p50", "p90", "p99", "Max"},
		// Stated rather than left empty, so the vocabulary a record may use
		// is written down in one place and a new description added to the
		// matrix without a translation is visible here.
		CallDetail: map[string]string{
			detailSmallestListing: "smallest listing",
			detailWholeSurface:    "whole surface",
		},
		SurfaceNote: "Resident set in MiB, times in milliseconds.",
		FigureAlt: map[string]string{
			"memory":      "Grouped bars comparing resident memory across the dynamic, meta and individual surfaces on both transports.",
			"memory-ramp": "Lines showing resident memory growing as each new credential builds its own catalog in the HTTP pool.",
			"startup":     "Grouped bars on a log scale comparing process readiness, the first cold tools/list and a warm one.",
			"latency":     "Grouped bars on a log scale comparing resources/list, tools/call and tools/list latency across transports and surfaces.",
		},
		TableCaption: map[string]string{
			"summary": "Memory, goroutines and processor time per scenario",
			"startup": "What a client waits for, per scenario",
			"latency": "Latency percentiles per method",
		},
	}
}

// spanishLabels is the wording of the Spanish page.
//
//nolint:misspell,dupl // Spanish prose against a US English dictionary; one bundle per language, identical in shape by definition
func spanishLabels() labels {
	//#nosec G101 -- user-facing wording, not a credential
	return labels{
		Code:            "es",
		MemoryTitle:     "Memoria residente por superficie de herramientas",
		MemorySubtitle:  "stdio mantiene un catálogo por proceso; HTTP mantiene uno por credencial en el pool",
		MemoryY:         "Conjunto residente (MiB)",
		MemoryStdioOne:  "stdio, un proceso",
		MemoryHTTPOne:   "HTTP, una credencial",
		MemoryHTTPAll:   "HTTP, %d credenciales",
		MemoryThreshold: "512 MiB",
		RampTitle:       "Memoria residente según llegan credenciales",
		RampSubtitle:    "Modo HTTP: cada token distinto construye su propio catálogo en el pool",
		RampX:           "credenciales activas (entradas del pool)",
		StartupTitle:    "Lo que espera un cliente",
		StartupSubtitle: "Modo HTTP, escala logarítmica: el catálogo se construye en el primer tools/list, no al arrancar",
		StartupY:        "milisegundos (escala logarítmica)",
		StartupReady:    "proceso listo",
		StartupCold:     "primer tools/list (en frío)",
		StartupWarm:     "tools/list ya en caliente (p50)",
		LatencyTitle:    "Latencia de petición por método",
		LatencySubtitle: "barra sólida p50, extensión tenue p99, escala logarítmica",
		MeasuredOn:      "Medido en %s, compilación %s, %s. %d rondas por método, conjunto residente muestreado cada %d ms.",
		HostCPUs:        "CPU lógicas",
		HostRAM:         "GiB de RAM",
		HostKernel:      "núcleo",
		SummaryHead: []string{
			"Escenario", "Clientes", "En reposo", "Un cliente", "Todos los clientes",
			"Por cliente extra", "Pico", "Goroutines", "CPU, % de un núcleo",
		},
		StartupHead: []string{
			"Escenario", "Proceso listo", "Primer tools/list", "tools/list en caliente (p50)", "Tamaño de tools/list",
		},
		LatencyHead: []string{"Escenario", "Método", "Llamada", "p50", "p90", "p99", "Máx"},
		CallDetail: map[string]string{
			detailSmallestListing: "el listado más pequeño",
			detailWholeSurface:    "la superficie completa",
		},
		SurfaceNote: "Conjunto residente en MiB, tiempos en milisegundos.",
		FigureAlt: map[string]string{
			"memory":      "Barras agrupadas que comparan la memoria residente de las superficies dynamic, meta e individual en ambos transportes.",
			"memory-ramp": "Líneas que muestran cómo crece la memoria residente cuando cada credencial nueva construye su catálogo en el pool HTTP.",
			"startup":     "Barras agrupadas en escala logarítmica que comparan el arranque del proceso, el primer tools/list en frío y uno en caliente.",
			"latency":     "Barras agrupadas en escala logarítmica que comparan la latencia de resources/list, tools/call y tools/list por transporte y superficie.",
		},
		TableCaption: map[string]string{
			"summary": "Memoria, goroutines y tiempo de procesador por escenario",
			"startup": "Lo que espera un cliente, por escenario",
			"latency": "Percentiles de latencia por método",
		},
	}
}

// surfaceOrder is the order surfaces appear in every figure and table: least
// registered first, which is also the order the documentation introduces them.
var surfaceOrder = []string{surfaceDynamic, surfaceMeta, surfaceIndividual}

// buildFigures assembles every figure for one language, in a fixed order.
// A figure the record holds no measurements for is left out rather than
// written empty, because an SVG with axes and no bars does not read as "not
// measured", it reads as zero.
func buildFigures(run *Run, l labels) []figure {
	memory, ramp := memorySpec(run, l), rampSpec(run, l)
	startup, latency := startupSpec(run, l), latencySpec(run, l)

	var out []figure
	add := func(name string, series int, render func(palette) string) {
		if series > 0 {
			out = append(out, figure{Name: name, Render: render})
		}
	}
	add("memory", len(memory.Series), func(p palette) string { return renderBars(p, memory) })
	add("memory-ramp", len(ramp.Series), func(p palette) string { return renderLines(p, ramp) })
	add("startup", len(startup.Series), func(p palette) string { return renderBars(p, startup) })
	add("latency", len(latency.Series), func(p palette) string { return renderBars(p, latency) })
	return out
}

// presentSurfaces keeps the surfaces this record actually measured, so a
// partial run still draws rather than inventing empty categories.
func presentSurfaces(run *Run) []string {
	var out []string
	for _, surface := range surfaceOrder {
		for _, scenario := range run.Scenarios {
			if scenario.Surface == surface && !scenario.Telemetry {
				out = append(out, surface)
				break
			}
		}
	}
	return out
}

// baseScenario finds the telemetry-off scenario for a transport and surface,
// which is what every figure is drawn from: telemetry is a separate comparison
// and would otherwise be mixed into the baseline.
func baseScenario(run *Run, transport, surface string) (Scenario, bool) {
	for _, scenario := range run.Scenarios {
		if scenario.Transport == transport && scenario.Surface == surface && !scenario.Telemetry {
			return scenario, true
		}
	}
	return Scenario{}, false
}

// memorySpec is the sizing figure: what one process costs on each transport,
// and what a shared HTTP deployment costs with every credential attached.
func memorySpec(run *Run, l labels) barSpec {
	surfaces := presentSurfaces(run)
	stdioOne := barSeries{Label: l.MemoryStdioOne}
	httpOne := barSeries{Label: l.MemoryHTTPOne}
	httpAll := barSeries{Label: l.MemoryHTTPAll}

	credentials := 0
	stdioComplete, httpComplete := true, true
	for _, surface := range surfaces {
		stdio, stdioOK := baseScenario(run, transportStdio, surface)
		stdioComplete = stdioComplete && stdioOK
		stdioOne.Values = append(stdioOne.Values, stdio.Memory.OneClientMiB)

		httpScenario, httpOK := baseScenario(run, transportHTTP, surface)
		httpComplete = httpComplete && httpOK
		httpOne.Values = append(httpOne.Values, httpScenario.Memory.OneClientMiB)
		httpAll.Values = append(httpAll.Values, httpScenario.Memory.AllClientsMiB)
		credentials = max(credentials, httpScenario.Clients)
	}
	httpAll.Label = fmt.Sprintf(l.MemoryHTTPAll, credentials)

	// A record restricted to one transport still names every surface, so a
	// series kept here regardless would draw a bar of zero for the transport
	// nobody measured, labeled as though it had been measured. Notably it
	// would read "HTTP, 0 credentials", which is a claim about the server
	// rather than about the run. A missing measurement is missing; zero is a
	// measurement.
	var series []barSeries
	if stdioComplete {
		series = append(series, stdioOne)
	}
	if httpComplete {
		series = append(series, httpOne, httpAll)
	}

	return barSpec{
		Title:      l.MemoryTitle,
		Subtitle:   l.MemorySubtitle,
		YAxis:      l.MemoryY,
		Categories: surfaces,
		Series:     series,
		Format:     func(v float64) string { return fmt.Sprintf("%.0f", v) },
		Threshold:  &thresholdLine{Value: 512, Label: l.MemoryThreshold},
	}
}

// rampSpec is the figure that answers "how many clients fit": one line per
// surface, one point per credential admitted.
func rampSpec(run *Run, l labels) lineSpec {
	var series []lineSeries
	for _, surface := range presentSurfaces(run) {
		scenario, ok := baseScenario(run, transportHTTP, surface)
		if !ok || len(scenario.Ramp) == 0 {
			continue
		}
		line := lineSeries{Label: surface}
		for _, point := range scenario.Ramp {
			line.X = append(line.X, float64(point.Client))
			line.Y = append(line.Y, point.RSSMiB)
		}
		series = append(series, line)
	}
	return lineSpec{
		Title:     l.RampTitle,
		Subtitle:  l.RampSubtitle,
		XAxis:     l.RampX,
		YAxis:     l.MemoryY,
		Series:    series,
		Format:    func(v float64) string { return fmt.Sprintf("%.0f", v) },
		Threshold: &thresholdLine{Value: 512, Label: l.MemoryThreshold},
	}
}

// startupSpec is the figure that makes registration visible: the process is
// ready in milliseconds and the surface behind it is not.
func startupSpec(run *Run, l labels) barSpec {
	surfaces := presentSurfaces(run)
	ready := barSeries{Label: l.StartupReady}
	cold := barSeries{Label: l.StartupCold}
	warm := barSeries{Label: l.StartupWarm}
	// Every category has to be measured for this figure to mean anything: the
	// three bars are one surface's timings beside another's, so a surface
	// filled in with zeros would read as a surface that starts instantly.
	for _, surface := range surfaces {
		scenario, ok := baseScenario(run, transportHTTP, surface)
		if !ok {
			return barSpec{}
		}
		ready.Values = append(ready.Values, scenario.Startup.ProcessReadyMs)
		cold.Values = append(cold.Values, scenario.Startup.FirstListMs)
		warm.Values = append(warm.Values, scenario.Startup.WarmListMs)
	}
	return barSpec{
		Title:      l.StartupTitle,
		Subtitle:   l.StartupSubtitle,
		YAxis:      l.StartupY,
		Categories: surfaces,
		Series:     []barSeries{ready, cold, warm},
		Log:        true,
		Format:     msLabel,
	}
}

// latencySpec compares the transports on the cheapest method and the surfaces
// on the most expensive one, in the same picture.
func latencySpec(run *Run, l labels) barSpec {
	methods := []string{"resources/list", "tools/call", "tools/list"}
	wanted := []struct {
		transport string
		surface   string
	}{
		{transportStdio, surfaceDynamic},
		{transportHTTP, surfaceDynamic},
		{transportHTTP, surfaceIndividual},
	}

	var series []barSeries
	for _, want := range wanted {
		scenario, ok := baseScenario(run, want.transport, want.surface)
		if !ok {
			continue
		}
		entry := barSeries{Label: want.transport + ", " + want.surface}
		for _, method := range methods {
			latency, found := scenario.latency(method)
			if !found {
				entry.Values = append(entry.Values, 0)
				entry.High = append(entry.High, 0)
				continue
			}
			entry.Values = append(entry.Values, latency.P50)
			entry.High = append(entry.High, latency.P99)
		}
		series = append(series, entry)
	}

	return barSpec{
		Title:      l.LatencyTitle,
		Subtitle:   l.LatencySubtitle,
		YAxis:      l.StartupY,
		Categories: methods,
		Series:     series,
		Log:        true,
		Format:     msLabel,
	}
}

// msLabel prints a millisecond figure at a precision that suits its size: a
// sub-millisecond ping and a six-second cold start share an axis here.
func msLabel(v float64) string {
	switch {
	case v == 0:
		return "0"
	case v < 1:
		return fmt.Sprintf("%.2f", v)
	case v < 10:
		return fmt.Sprintf("%.1f", v)
	default:
		return fmt.Sprintf("%.0f", v)
	}
}

// scenarioLabel names a scenario in a table row.
func scenarioLabel(s Scenario) string {
	label := s.Transport + ", " + s.Surface
	if s.Telemetry {
		label += ", telemetry"
	}
	return label
}

// bytesLabel renders a payload size the way an operator reads it.
func bytesLabel(bytes int) string {
	switch {
	case bytes >= 1024*1024:
		return fmt.Sprintf("%.1f MB", float64(bytes)/(1024*1024))
	case bytes >= 1024:
		return fmt.Sprintf("%.0f KB", float64(bytes)/1024)
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

// joinNonEmpty joins the parts that have content, which keeps a note from
// ending in a dangling separator.
func joinNonEmpty(sep string, parts ...string) string {
	kept := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			kept = append(kept, part)
		}
	}
	return strings.Join(kept, sep)
}
