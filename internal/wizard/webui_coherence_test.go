package wizard

import (
	"regexp"
	"sort"
	"strings"
	"testing"
)

// loadEmbeddedHTML returns the embedded index.html content as a string.
// Fails the test if the asset cannot be read.
func loadEmbeddedHTML(t *testing.T) string {
	t.Helper()
	data, err := webAssets.ReadFile("webui_assets/index.html")
	if err != nil {
		t.Fatalf("reading embedded HTML: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("embedded HTML is empty")
	}
	return string(data)
}

// TestWebUI_ElementIDs_ReferencedInJS_ExistInHTML verifies that every
// document.getElementById('X') call in the JavaScript has a matching
// id="X" attribute in the HTML markup. A mismatch means the JS will get
// null and crash at runtime — which is exactly what caused the original
// wizard bug (truncated button elements lost their id attributes).
func TestWebUI_ElementIDs_ReferencedInJS_ExistInHTML(t *testing.T) {
	html := loadEmbeddedHTML(t)

	// Extract all getElementById('X') and getElementById("X") calls
	jsIDPattern := regexp.MustCompile(`getElementById\(['"]([^'"]+)['"]\)`)
	jsMatches := jsIDPattern.FindAllStringSubmatch(html, -1)
	if len(jsMatches) == 0 {
		t.Fatal("no getElementById calls found in HTML — test is broken or HTML structure changed")
	}

	jsIDs := make(map[string]bool)
	for _, m := range jsMatches {
		jsIDs[m[1]] = true
	}

	// Extract all id="X" attributes from the HTML
	htmlIDPattern := regexp.MustCompile(`id="([^"]+)"`)
	htmlMatches := htmlIDPattern.FindAllStringSubmatch(html, -1)
	htmlIDs := make(map[string]bool)
	for _, m := range htmlMatches {
		htmlIDs[m[1]] = true
	}

	// Every JS-referenced ID must exist in HTML
	var missing []string
	for id := range jsIDs {
		if !htmlIDs[id] {
			missing = append(missing, id)
		}
	}
	sort.Strings(missing)

	if len(missing) > 0 {
		t.Errorf("JavaScript references %d element ID(s) not found in HTML: %v", len(missing), missing)
		t.Log("This usually means a button or element tag is malformed or truncated.")
	}
}

// TestWebUI_APIEndpoints_InJS_MatchGoHandlers verifies that every
// fetch('/api/...') call in the JavaScript targets an endpoint that is
// registered in the Go HTTP mux (RunWebUI). If a new endpoint is added
// in Go or JS without updating the other, this test will catch it.
func TestWebUI_APIEndpoints_InJS_MatchGoHandlers(t *testing.T) {
	html := loadEmbeddedHTML(t)

	// Extract all fetch('/api/...') endpoints from JavaScript
	fetchPattern := regexp.MustCompile(`fetch\(['"](/api/[^'"]+)['"]\s*[,)]`)
	fetchMatches := fetchPattern.FindAllStringSubmatch(html, -1)
	if len(fetchMatches) == 0 {
		t.Fatal("no fetch('/api/...') calls found in HTML — test is broken or HTML structure changed")
	}

	jsEndpoints := make(map[string]bool)
	for _, m := range fetchMatches {
		jsEndpoints[m[1]] = true
	}

	// These are the API endpoints registered in RunWebUI's mux.
	// If you add a new handler, add it here too.
	goEndpoints := map[string]bool{
		"/api/defaults":       true,
		"/api/pick-directory": true,
		"/api/configure":      true,
	}

	// Every JS endpoint must have a Go handler
	for ep := range jsEndpoints {
		if !goEndpoints[ep] {
			t.Errorf("JavaScript fetches %q but no Go handler is registered for it", ep)
		}
	}

	// Every Go endpoint should be used by JS (optional but helpful)
	for ep := range goEndpoints {
		if !jsEndpoints[ep] {
			t.Errorf("Go handler %q is registered but never called from JavaScript", ep)
		}
	}
}

// TestWebUI_DefaultsJSONFields_UsedInJS verifies that the JavaScript
// accesses only JSON fields that exist in the defaultsResponse struct.
// This prevents silent field name mismatches (e.g., JS reads "version"
// but Go sends "ver") that cause the UI to show empty values.
func TestWebUI_DefaultsJSONFields_UsedInJS(t *testing.T) {
	html := loadEmbeddedHTML(t)

	// Extract JavaScript portion (everything inside <script>...</script>)
	scriptStart := strings.Index(html, "<script")
	scriptEnd := strings.LastIndex(html, "</script>")
	if scriptStart < 0 || scriptEnd < 0 {
		t.Fatal("cannot find <script> block in HTML")
	}
	jsCode := html[scriptStart:scriptEnd]

	// Extract defaults.X field accesses from JS
	fieldPattern := regexp.MustCompile(`defaults\.([a-z_]+)`)
	fieldMatches := fieldPattern.FindAllStringSubmatch(jsCode, -1)
	if len(fieldMatches) == 0 {
		t.Fatal("no defaults.X field accesses found in JavaScript")
	}

	jsFields := make(map[string]bool)
	for _, m := range fieldMatches {
		jsFields[m[1]] = true
	}

	// JSON field names from defaultsResponse struct tags.
	// clientResponse sub-fields accessed as c.X are also included.
	goFields := map[string]bool{
		// defaultsResponse fields
		"version":           true,
		"installed_version": true,
		"install_path":      true,
		"gitlab_url":        true,
		"has_existing":      true,
		"masked_token":      true,
		"skip_tls_verify":   true,
		"clients":           true,
		// clientResponse fields (accessed via c.X in forEach)
		"name":             true,
		"config_path":      true,
		"display_only":     true,
		"default_selected": true,
	}

	var unknown []string
	for field := range jsFields {
		if !goFields[field] {
			unknown = append(unknown, field)
		}
	}
	sort.Strings(unknown)

	if len(unknown) > 0 {
		t.Errorf("JavaScript reads %d defaults field(s) not in Go struct: %v", len(unknown), unknown)
	}
}

// TestWebUI_ConfigureRequestFields_InJS_MatchGoStruct verifies that the
// JSON body sent by the configure() function in JavaScript contains
// exactly the fields expected by the configureRequest Go struct.
func TestWebUI_ConfigureRequestFields_InJS_MatchGoStruct(t *testing.T) {
	html := loadEmbeddedHTML(t)

	// Extract the body object from the configure() function.
	// It's between "const body = {" and the closing "};"
	bodyStart := strings.Index(html, "const body = {")
	if bodyStart < 0 {
		t.Fatal("cannot find 'const body = {' in HTML")
	}

	// Find the matching closing brace
	depth := 0
	bodyEnd := -1
	for i := bodyStart + len("const body = "); i < len(html); i++ {
		switch html[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				bodyEnd = i + 1
			}
		}
		if bodyEnd > 0 {
			break
		}
	}
	if bodyEnd < 0 {
		t.Fatal("cannot find closing brace for body object in configure()")
	}

	bodyBlock := html[bodyStart:bodyEnd]

	// Extract field names from the JS object literal (key: value pairs)
	jsFieldPattern := regexp.MustCompile(`(?m)^\s+([a-z_]+)\s*:`)
	jsFieldMatches := jsFieldPattern.FindAllStringSubmatch(bodyBlock, -1)
	if len(jsFieldMatches) == 0 {
		t.Fatal("no fields found in JavaScript body object")
	}

	jsFields := make(map[string]bool)
	for _, m := range jsFieldMatches {
		jsFields[m[1]] = true
	}

	// Expected fields from configureRequest struct JSON tags
	goFields := map[string]bool{
		"install_path":     true,
		"gitlab_url":       true,
		"gitlab_token":     true,
		"skip_tls_verify":  true,
		"meta_tools":       true,
		"auto_update":      true,
		"yolo_mode":        true,
		"log_level":        true,
		"selected_clients": true,
	}

	// Fields in JS but not in Go
	var jsOnly []string
	for f := range jsFields {
		if !goFields[f] {
			jsOnly = append(jsOnly, f)
		}
	}
	sort.Strings(jsOnly)
	if len(jsOnly) > 0 {
		t.Errorf("JavaScript sends %d field(s) not in Go configureRequest: %v", len(jsOnly), jsOnly)
	}

	// Fields in Go but not in JS
	var goOnly []string
	for f := range goFields {
		if !jsFields[f] {
			goOnly = append(goOnly, f)
		}
	}
	sort.Strings(goOnly)
	if len(goOnly) > 0 {
		t.Errorf("Go configureRequest has %d field(s) not sent by JavaScript: %v", len(goOnly), goOnly)
	}
}

// TestWebUI_Buttons_HaveProperStructure verifies that every button element
// in the HTML has a valid id attribute, proper opening/closing tags, and
// visible text content. This is the regression test for the original bug
// where truncated button elements broke the wizard.
func TestWebUI_Buttons_HaveProperStructure(t *testing.T) {
	html := loadEmbeddedHTML(t)

	// Interactive buttons that JavaScript binds to (not toggle sliders)
	requiredButtons := []struct {
		id    string
		class string
	}{
		{id: "browseBtn", class: "btn-browse"},
		{id: "selectAllBtn", class: "select-all-btn"},
		{id: "configureBtn", class: "btn-primary"},
		{id: "closeBtn", class: "btn-primary"},
	}

	for _, btn := range requiredButtons {
		t.Run(btn.id, func(t *testing.T) {
			// Must have id="X" in a <button> element
			idAttr := `id="` + btn.id + `"`
			if !strings.Contains(html, idAttr) {
				t.Fatalf("button id=%q not found in HTML", btn.id)
			}

			// Extract the full <button...>...</button> element
			btnPattern := regexp.MustCompile(`<button[^>]*id="` + regexp.QuoteMeta(btn.id) + `"[^>]*>(.*?)</button>`)
			match := btnPattern.FindStringSubmatch(html)
			if match == nil {
				t.Fatalf("button id=%q is missing closing </button> tag or has malformed markup", btn.id)
			}

			// Must have visible text content
			textContent := strings.TrimSpace(match[1])
			if textContent == "" {
				t.Errorf("button id=%q has no visible text content", btn.id)
			}

			// Must have the expected class
			fullTag := match[0]
			if !strings.Contains(fullTag, btn.class) {
				t.Errorf("button id=%q missing expected class %q in: %s", btn.id, btn.class, fullTag)
			}
		})
	}
}

// TestWebUI_InstalledVersion_ElementExists verifies the HTML contains the
// installedVersion element that JavaScript populates with version info.
func TestWebUI_InstalledVersion_ElementExists(t *testing.T) {
	html := loadEmbeddedHTML(t)

	if !strings.Contains(html, `id="installedVersion"`) {
		t.Error("HTML is missing the installedVersion element needed for showing the installed binary version")
	}
}
