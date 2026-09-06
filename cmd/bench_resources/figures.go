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
	"time"
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

	MemoryTitle    string
	MemorySubtitle string
	MemoryY        string
	// SurfaceX and MethodX name what a bar chart's categories are, since
	// "dynamic" and "tools/call" only read as a tool surface and an MCP
	// method to somebody who already knows.
	SurfaceX        string
	MethodX         string
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

	// The concurrency series: three figures over the credential count, the
	// per-step table, and the sentence that says where a series stopped.
	SeriesMemoryTitle     string
	SeriesMemorySubtitle  string
	SeriesLatencyTitle    string
	SeriesLatencySubtitle string
	SeriesCPUTitle        string
	SeriesCPUSubtitle     string
	SeriesCPUY            string
	SeriesX               string
	SeriesBudget          string
	SeriesStopped         string
	SeriesP50             string
	SeriesP99             string
	SeriesHead            []string
	// SeriesSettledHeap and SeriesSettledRSS head the two columns a step
	// carries only when its series took a settled reading, spliced in after
	// the resident columns rather than kept in SeriesHead, which describes the
	// figures every series has.
	SeriesSettledHeap string
	SeriesSettledRSS  string
	// SeriesSlopes and SeriesLoadSlopeOnly are the sentence under a series
	// table. Both name what their figures measure: one number per credential
	// is read as what a credential costs, and the resident one is the
	// credential and its requests in flight together.
	SeriesSlopes        string
	SeriesLoadSlopeOnly string
	SeriesCaption       string
	SeriesBudgetClause  string
	SeriesNoBudget      string
	SeriesComplete      string
	SeriesStopBudget    string
	SeriesStopLatency   string
	SeriesStopFailure   string

	MeasuredOn string
	// ChartProvenance is the same claim as MeasuredOn, short enough to sit
	// along a figure's bottom edge: the machine, the build and the date. A
	// chart gets embedded, screenshotted and quoted away from the page that
	// states those, and a memory curve with no machine attached is a number
	// nobody can act on.
	ChartProvenance string
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
		Code:                  "en",
		MemoryTitle:           "Resident memory by tool surface",
		MemorySubtitle:        "stdio holds one catalog per process, HTTP one per configuration shared by every credential",
		MemoryY:               "Resident set (MiB)",
		SurfaceX:              "tool surface",
		MethodX:               "MCP method",
		MemoryStdioOne:        "stdio, one process",
		MemoryHTTPOne:         "HTTP, one credential",
		MemoryHTTPAll:         "HTTP, %d credentials",
		MemoryThreshold:       "512 MiB",
		RampTitle:             "Resident memory as credentials arrive",
		RampSubtitle:          "HTTP mode, credentials admitted one at a time: the first builds the catalog, the rest find it built",
		RampX:                 "live credentials (pool entries)",
		StartupTitle:          "What a client waits for",
		StartupSubtitle:       "HTTP mode, log scale: the catalog is built on the first tools/list, not at startup",
		StartupY:              "milliseconds (log scale)",
		StartupReady:          "process ready",
		StartupCold:           "first tools/list (cold)",
		StartupWarm:           "tools/list once warm (p50)",
		LatencyTitle:          "Request latency by method",
		LatencySubtitle:       "solid bar p50, faint extension p99, log scale",
		SeriesMemoryTitle:     "Resident memory as credentials accumulate",
		SeriesMemorySubtitle:  "HTTP mode, one process: each step's peak, the memory budget, and where a series stopped",
		SeriesLatencyTitle:    "tools/call latency as credentials accumulate",
		SeriesLatencySubtitle: "solid line p50, dashed line p99, both axes on a log scale",
		SeriesCPUTitle:        "Processor time per call as credentials accumulate",
		SeriesCPUSubtitle:     "CPU time the server consumed over the steady phase, divided by the calls that completed",
		SeriesCPUY:            "CPU milliseconds per call",
		SeriesX:               "live credentials (log scale)",
		SeriesBudget:          "budget %.0f MiB",
		SeriesStopped:         "%s: stopped at %d",
		SeriesP50:             "%s p50",
		SeriesP99:             "%s p99",
		SeriesHead: []string{
			"Credentials", "Resident, mean", "Resident, peak", "CPU per call", "Calls",
			"tools/call p50", "tools/call p99", "tools/list p50", "tools/list p99", "Goroutines",
		},
		SeriesSettledHeap: "Settled heap",
		SeriesSettledRSS:  "Settled resident",
		SeriesSlopes: "Fitted across these steps: the peak resident set under load grows %.2f MiB per credential, " +
			"and the settled live heap, read with the load stopped and a collection forced, grows %.1f KiB per credential. " +
			"The first is what a credential costs while it and every other one is calling; the second is what it costs to hold. " +
			"The settled resident set lags both, because Go returns freed pages to the operating system on its own schedule.",
		SeriesLoadSlopeOnly: "Fitted across these steps, the peak resident set under load grows %.2f MiB per credential. " +
			"That is a credential together with the requests it keeps in flight, not what a credential costs to hold: " +
			"this record carries no settled reading.",
		SeriesCaption:      "%s, %s surface: %d in flight per credential, %.0f s per step, %s",
		SeriesBudgetClause: "memory budget %.0f MiB",
		SeriesNoBudget:     "no memory budget",
		SeriesComplete:     "Every planned step ran, up to %d credentials.",
		SeriesStopBudget:   "Stopped at %d credentials: the next step (%d) was estimated at %.0f MiB against a budget of %.0f MiB.",
		SeriesStopLatency:  "Stopped at %d credentials: the tools/call p99 reached %.0f ms, above the %d ms ceiling.",
		SeriesStopFailure:  "Stopped at %d credentials: admitting the credentials of the next step (%d) failed: %s.",
		MeasuredOn:         "Measured on %s, build %s, %s. %d rounds per method, resident set sampled every %d ms.",
		ChartProvenance:    "Measured on %s. Build %s, %s.",
		HostCPUs:           "logical CPUs",
		HostRAM:            "GiB RAM",
		HostKernel:         "kernel",
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
			figureMemory:        "Grouped bars comparing resident memory across the dynamic, meta and individual surfaces on both transports.",
			figureMemoryRamp:    "Lines showing resident memory growing as each new credential builds its own catalog in the HTTP pool.",
			figureStartup:       "Grouped bars on a log scale comparing process readiness, the first cold tools/list and a warm one.",
			figureLatency:       "Grouped bars on a log scale comparing resources/list, tools/call and tools/list latency across transports and surfaces.",
			figureSeriesMemory:  "Lines on a log scale of credentials showing each surface's peak resident memory per step, the memory budget, and the count each series stopped at.",
			figureSeriesLatency: "Lines on log scales showing the tools/call p50 and p99 per surface as the credential count grows.",
			figureSeriesCPU:     "Lines on a log scale of credentials showing the processor time per call for each surface.",
		},
		TableCaption: map[string]string{
			"summary": "Memory, goroutines and processor time per scenario",
			"startup": "What a client waits for, per scenario",
			"latency": "Latency percentiles per method",
			"series":  "Concurrency series",
		},
	}
}

// spanishLabels is the wording of the Spanish page.
//
//nolint:misspell,dupl // Spanish prose against a US English dictionary; one bundle per language, identical in shape by definition
func spanishLabels() labels {
	//#nosec G101 -- user-facing wording, not a credential
	return labels{
		Code:                  "es",
		MemoryTitle:           "Memoria residente por superficie de herramientas",
		MemorySubtitle:        "stdio mantiene un catálogo por proceso, HTTP uno por configuración compartido por todas las credenciales",
		MemoryY:               "Conjunto residente (MiB)",
		SurfaceX:              "superficie de herramientas",
		MethodX:               "método MCP",
		MemoryStdioOne:        "stdio, un proceso",
		MemoryHTTPOne:         "HTTP, una credencial",
		MemoryHTTPAll:         "HTTP, %d credenciales",
		MemoryThreshold:       "512 MiB",
		RampTitle:             "Memoria residente según llegan credenciales",
		RampSubtitle:          "Modo HTTP, credenciales admitidas de una en una: la primera construye el catálogo y el resto lo encuentra construido",
		RampX:                 "credenciales activas (entradas del pool)",
		StartupTitle:          "Lo que espera un cliente",
		StartupSubtitle:       "Modo HTTP, escala logarítmica: el catálogo se construye en el primer tools/list, no al arrancar",
		StartupY:              "milisegundos (escala logarítmica)",
		StartupReady:          "proceso listo",
		StartupCold:           "primer tools/list (en frío)",
		StartupWarm:           "tools/list ya en caliente (p50)",
		LatencyTitle:          "Latencia de petición por método",
		LatencySubtitle:       "barra sólida p50, extensión tenue p99, escala logarítmica",
		SeriesMemoryTitle:     "Memoria residente según se acumulan credenciales",
		SeriesMemorySubtitle:  "Modo HTTP, un proceso: el pico de cada paso, el presupuesto de memoria y dónde se detuvo cada serie",
		SeriesLatencyTitle:    "Latencia de tools/call según se acumulan credenciales",
		SeriesLatencySubtitle: "línea continua p50, discontinua p99, ambos ejes en escala logarítmica",
		SeriesCPUTitle:        "Tiempo de procesador por llamada según se acumulan credenciales",
		SeriesCPUSubtitle:     "tiempo de CPU consumido por el servidor en la fase estable, dividido entre las llamadas completadas",
		SeriesCPUY:            "milisegundos de CPU por llamada",
		SeriesX:               "credenciales activas (escala logarítmica)",
		SeriesBudget:          "presupuesto %.0f MiB",
		SeriesStopped:         "%s: detenida en %d",
		SeriesP50:             "%s p50",
		SeriesP99:             "%s p99",
		SeriesHead: []string{
			"Credenciales", "Residente, media", "Residente, pico", "CPU por llamada", "Llamadas",
			"tools/call p50", "tools/call p99", "tools/list p50", "tools/list p99", "Goroutines",
		},
		SeriesSettledHeap: "Heap en reposo",
		SeriesSettledRSS:  "Residente en reposo",
		SeriesSlopes: "Ajustado sobre estos pasos: el pico de conjunto residente bajo carga crece %.2f MiB por credencial, " +
			"y el heap vivo en reposo, leído con la carga detenida y una recolección forzada, crece %.1f KiB por credencial. " +
			"Lo primero es lo que cuesta una credencial mientras ella y todas las demás están llamando; lo segundo es lo que cuesta mantenerla. " +
			"El conjunto residente en reposo va por detrás de ambos, porque Go devuelve al sistema operativo las páginas liberadas según su propio calendario.",
		SeriesLoadSlopeOnly: "Ajustado sobre estos pasos, el pico de conjunto residente bajo carga crece %.2f MiB por credencial. " +
			"Eso es una credencial junto con las peticiones que mantiene en vuelo, no lo que cuesta mantener una credencial: " +
			"este registro no contiene ninguna lectura en reposo.",
		SeriesCaption:      "%s, superficie %s: %d en vuelo por credencial, %.0f s por paso, %s",
		SeriesBudgetClause: "presupuesto de memoria de %.0f MiB",
		SeriesNoBudget:     "sin presupuesto de memoria",
		SeriesComplete:     "Se ejecutaron todos los pasos previstos, hasta %d credenciales.",
		SeriesStopBudget:   "Detenida en %d credenciales: el siguiente paso (%d) se estimó en %.0f MiB frente a un presupuesto de %.0f MiB.",
		SeriesStopLatency:  "Detenida en %d credenciales: el p99 de tools/call alcanzó %.0f ms, por encima del techo de %d ms.",
		SeriesStopFailure:  "Detenida en %d credenciales: no se pudieron admitir las credenciales del siguiente paso (%d): %s.",
		MeasuredOn:         "Medido en %s, compilación %s, %s. %d rondas por método, conjunto residente muestreado cada %d ms.",
		ChartProvenance:    "Medido en %s. Compilación %s, %s.",
		HostCPUs:           "CPU lógicas",
		HostRAM:            "GiB de RAM",
		HostKernel:         "núcleo",
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
			figureMemory:        "Barras agrupadas que comparan la memoria residente de las superficies dynamic, meta e individual en ambos transportes.",
			figureMemoryRamp:    "Líneas que muestran cómo crece la memoria residente cuando cada credencial nueva construye su catálogo en el pool HTTP.",
			figureStartup:       "Barras agrupadas en escala logarítmica que comparan el arranque del proceso, el primer tools/list en frío y uno en caliente.",
			figureLatency:       "Barras agrupadas en escala logarítmica que comparan la latencia de resources/list, tools/call y tools/list por transporte y superficie.",
			figureSeriesMemory:  "Líneas sobre una escala logarítmica de credenciales con el pico de memoria residente de cada superficie por paso, el presupuesto de memoria y el número de credenciales en que se detuvo cada serie.",
			figureSeriesLatency: "Líneas en escalas logarítmicas con el p50 y el p99 de tools/call por superficie según crece el número de credenciales.",
			figureSeriesCPU:     "Líneas sobre una escala logarítmica de credenciales con el tiempo de procesador por llamada de cada superficie.",
		},
		TableCaption: map[string]string{
			"summary": "Memoria, goroutines y tiempo de procesador por escenario",
			"startup": "Lo que espera un cliente, por escenario",
			"latency": "Percentiles de latencia por método",
			"series":  "Serie de concurrencia",
		},
	}
}

// surfaceOrder is the order surfaces appear in every figure and table: least
// registered first, which is also the order the documentation introduces them.
var surfaceOrder = []string{surfaceDynamic, surfaceMeta, surfaceIndividual}

// The figure names, which are the SVG file names the pages embed and the keys
// of each language's alt-text table. Named because those three uses have to
// agree: a renamed figure that missed one of them would either lose its
// description or point a page at a file nobody writes.
const (
	figureMemory        = "memory"
	figureMemoryRamp    = "memory-ramp"
	figureStartup       = "startup"
	figureLatency       = "latency"
	figureSeriesMemory  = "series-memory"
	figureSeriesLatency = "series-latency"
	figureSeriesCPU     = "series-cpu"
)

// buildFigures assembles every figure for one language, in a fixed order.
// A figure the record holds no measurements for is left out rather than
// written empty, because an SVG with axes and no bars does not read as "not
// measured", it reads as zero. The series figures follow the same rule: a
// record from before the series, or a run that measured none, draws none.
func buildFigures(run *Run, l labels) []figure {
	memory, ramp := memorySpec(run, l), rampSpec(run, l)
	startup, latency := startupSpec(run, l), latencySpec(run, l)
	seriesMemory, seriesLatency, seriesCPU := seriesMemorySpec(run, l), seriesLatencySpec(run, l), seriesCPUSpec(run, l)

	// Every figure carries the same provenance, stamped here rather than in
	// each builder so a figure added later cannot be drawn without one.
	stamp := chartProvenance(run, l)
	memory.Provenance = stamp
	startup.Provenance = stamp
	latency.Provenance = stamp
	ramp.Provenance = stamp
	seriesMemory.Provenance = stamp
	seriesLatency.Provenance = stamp
	seriesCPU.Provenance = stamp

	var out []figure
	add := func(name string, series int, render func(palette) string) {
		if series > 0 {
			out = append(out, figure{Name: name, Render: render})
		}
	}
	add(figureMemory, len(memory.Series), func(p palette) string { return renderBars(p, memory) })
	add(figureMemoryRamp, len(ramp.Series), func(p palette) string { return renderLines(p, ramp) })
	add(figureStartup, len(startup.Series), func(p palette) string { return renderBars(p, startup) })
	add(figureLatency, len(latency.Series), func(p palette) string { return renderBars(p, latency) })
	add(figureSeriesMemory, len(seriesMemory.Series), func(p palette) string { return renderLines(p, seriesMemory) })
	add(figureSeriesLatency, len(seriesLatency.Series), func(p palette) string { return renderLines(p, seriesLatency) })
	add(figureSeriesCPU, len(seriesCPU.Series), func(p palette) string { return renderLines(p, seriesCPU) })
	return out
}

// chartProvenance is the line every figure carries along its bottom edge: the
// machine, the build and the day.
//
// It is deliberately shorter than the sentence under the "Measurements"
// heading. That sentence has a paragraph's width to spend and names the kernel,
// the installed memory, the round count and the sampling interval; this one has
// a chart's width, and what a reader needs from a chart read on its own is
// which machine and which build, not how the sampler was configured. The
// date rather than the timestamp for the same reason: two runs on one day are
// already told apart by the commit.
func chartProvenance(run *Run, l labels) string {
	if l.ChartProvenance == "" {
		return ""
	}
	return fmt.Sprintf(l.ChartProvenance, run.Host.describeShort(l), buildLabel(run), measurementDay(run))
}

// measurementDay trims a record's timestamp to its date, leaving anything that
// is not an RFC 3339 timestamp alone: a record written by hand for a test is
// still worth printing.
func measurementDay(run *Run) string {
	const dateLen = len(time.DateOnly)
	if len(run.GeneratedAt) >= dateLen {
		if _, err := time.Parse(time.DateOnly, run.GeneratedAt[:dateLen]); err == nil {
			return run.GeneratedAt[:dateLen]
		}
	}
	return run.GeneratedAt
}

// orderedSeries lists the record's series in surface order, which is the
// order every figure's legend and every table follows.
func orderedSeries(run *Run) []SeriesScenario {
	var out []SeriesScenario
	for _, surface := range surfaceOrder {
		for _, series := range run.Series {
			if series.Surface == surface && len(series.Steps) > 0 {
				out = append(out, series)
			}
		}
	}
	return out
}

// seriesLine draws one series as a line over its steps' credential counts,
// taking the Y of each step from pick.
func seriesLine(series SeriesScenario, label string, pick func(SeriesStep) float64) lineSeries {
	line := lineSeries{Label: label}
	for _, step := range series.Steps {
		line.X = append(line.X, float64(step.Clients))
		line.Y = append(line.Y, pick(step))
	}
	return line
}

// seriesMemorySpec is the sizing figure at scale: each surface's peak
// resident set per step, the budget the series planned against, and a
// marker where each stopped early.
//
// The surfaces share one chart rather than getting one each, because the
// comparison a reader makes is between surfaces at the same credential
// count, and the budget and the stops read the same on one axis.
func seriesMemorySpec(run *Run, l labels) lineSpec {
	spec := lineSpec{
		Title: l.SeriesMemoryTitle, Subtitle: l.SeriesMemorySubtitle,
		XAxis: l.SeriesX, YAxis: l.MemoryY, LogX: true,
		Format: func(v float64) string { return fmt.Sprintf("%.0f", v) },
	}
	for _, series := range orderedSeries(run) {
		spec.Series = append(spec.Series, seriesLine(series, series.Surface, func(s SeriesStep) float64 { return s.RSSPeakMiB }))
		if spec.Threshold == nil && series.BudgetMiB > 0 {
			spec.Threshold = &thresholdLine{Value: series.BudgetMiB, Label: fmt.Sprintf(l.SeriesBudget, series.BudgetMiB)}
		}
		if series.Stop != nil {
			spec.Markers = append(spec.Markers, lineMarker{
				X: float64(series.StoppedAt), Label: fmt.Sprintf(l.SeriesStopped, series.Surface, series.StoppedAt),
			})
		}
	}
	return spec
}

// seriesLatencySpec is where the latency knee shows: the tools/call median
// and tail per surface, on log axes, the tail dashed beside its median.
func seriesLatencySpec(run *Run, l labels) lineSpec {
	spec := lineSpec{
		Title: l.SeriesLatencyTitle, Subtitle: l.SeriesLatencySubtitle,
		XAxis: l.SeriesX, YAxis: l.StartupY, LogX: true, LogY: true, Format: msLabel,
	}
	for _, series := range orderedSeries(run) {
		median := seriesLine(series, fmt.Sprintf(l.SeriesP50, series.Surface), func(s SeriesStep) float64 { return s.CallP50Ms })
		median.Group = series.Surface
		tail := seriesLine(series, fmt.Sprintf(l.SeriesP99, series.Surface), func(s SeriesStep) float64 { return s.CallP99Ms })
		tail.Group = series.Surface
		tail.Dashed = true
		spec.Series = append(spec.Series, median, tail)
	}
	return spec
}

// seriesCPUSpec is the cost of a call at scale: processor milliseconds per
// completed call, per surface.
func seriesCPUSpec(run *Run, l labels) lineSpec {
	spec := lineSpec{
		Title: l.SeriesCPUTitle, Subtitle: l.SeriesCPUSubtitle,
		XAxis: l.SeriesX, YAxis: l.SeriesCPUY, LogX: true,
		Format: func(v float64) string { return fmt.Sprintf("%.2f", v) },
	}
	for _, series := range orderedSeries(run) {
		spec.Series = append(spec.Series, seriesLine(series, series.Surface, func(s SeriesStep) float64 { return s.CPUMsPerCall }))
	}
	return spec
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
		XAxis:      l.SurfaceX,
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
		XAxis:      l.SurfaceX,
		Categories: surfaces,
		Series:     []barSeries{ready, cold, warm},
		Log:        true,
		Format:     msLabel,
	}
}

// latencySpec compares the transports on the cheapest method and the surfaces
// on the most expensive one, in the same picture.
func latencySpec(run *Run, l labels) barSpec {
	methods := []string{methodResourcesList, methodToolsCall, methodToolsList}
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
		XAxis:      l.MethodX,
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
//
// The units are decimal, because the quantity is a response body counted in
// bytes on the wire rather than memory the kernel hands out in pages: nothing
// about a JSON document is measured in multiples of 1024. Every memory figure
// on the same pages is binary and says so, MiB and GiB, and the two conventions
// are kept apart deliberately. Until 2.8.0 this divided by 1024 while labeling
// the result MB, so a 3,235,932-byte tools/list was published as "3.1 MB" when
// 3.1 MiB was what had been computed.
func bytesLabel(bytes int) string {
	switch {
	case bytes >= 1000*1000:
		return fmt.Sprintf("%.1f MB", float64(bytes)/(1000*1000))
	case bytes >= 1000:
		return fmt.Sprintf("%.0f KB", float64(bytes)/1000)
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
