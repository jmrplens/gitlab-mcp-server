package wizard

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
)

// MergeServerEntry reads an existing JSON config file (or starts fresh),
// sets the server entry under the given rootKey, and writes the result back.
// It preserves all other keys in the file.
func MergeServerEntry(configPath, rootKey, serverName string, entry map[string]any) error {
	existing, err := readJSONFile(configPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading %s: %w", configPath, err)
	}
	if existing == nil {
		existing = make(map[string]any)
	}

	servers, ok := existing[rootKey].(map[string]any)
	if !ok {
		servers = make(map[string]any)
	}
	servers[serverName] = entry
	existing[rootKey] = servers

	return writeJSONFile(configPath, existing)
}

// readJSONFile reads and parses a JSON or JSONC file into a generic map.
// It strips single-line comments, block comments, and trailing commas
// to handle VS Code-style JSONC files (e.g., mcp.json).
func readJSONFile(path string) (map[string]any, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path is a known config file location, not user input
	if err != nil {
		return nil, err
	}
	cleaned := stripJSONC(data)
	var result map[string]any
	if err = json.Unmarshal(cleaned, &result); err != nil {
		return nil, fmt.Errorf("parsing JSON in %s: %w", path, err)
	}
	return result, nil
}

var trailingCommaRe = regexp.MustCompile(`,(\s*[}\]])`)

// stripJSONC removes // and /* */ comments and trailing commas from JSONC.
func stripJSONC(data []byte) []byte {
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF}) // BOM

	var out bytes.Buffer
	inString := false
	i, n := 0, len(data)

	for i < n {
		c := data[i]

		if c == '"' {
			backslashes := 0
			for j := i - 1; j >= 0 && data[j] == '\\'; j-- {
				backslashes++
			}
			if backslashes%2 == 0 {
				inString = !inString
			}
			out.WriteByte(c)
			i++
			continue
		}

		if inString {
			out.WriteByte(c)
			i++
			continue
		}

		// Single-line comment
		if c == '/' && i+1 < n && data[i+1] == '/' {
			i += 2
			for i < n && data[i] != '\n' {
				i++
			}
			continue
		}

		// Block comment
		if c == '/' && i+1 < n && data[i+1] == '*' {
			i += 2
			for i+1 < n {
				if data[i] == '*' && data[i+1] == '/' {
					i += 2
					break
				}
				i++
			}
			continue
		}

		out.WriteByte(c)
		i++
	}

	return trailingCommaRe.ReplaceAll(out.Bytes(), []byte("$1"))
}

// writeJSONFile marshals a map to indented JSON and writes it to path,
// creating parent directories as needed.
func writeJSONFile(path string, data map[string]any) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil { // #nosec G301 -- config dir needs execute permission
		return fmt.Errorf("creating directory %s: %w", dir, err)
	}

	out, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling JSON: %w", err)
	}
	out = append(out, '\n')

	perm := os.FileMode(0o644)
	if runtime.GOOS != "windows" {
		perm = 0o600
	}
	if err = os.WriteFile(path, out, perm); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}
