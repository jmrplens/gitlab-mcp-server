// Command audit_install_buttons checks the one-click install buttons against
// what the pages around them claim.
//
// A button's configuration travels inside its URL, base64 in every client this
// project links and percent-encoded on top of that in some. Nothing about it is
// visible in review, and no text search finds a flag inside it: removing
// "--http=false" from every example in the tree left eight buttons still
// registering it, because the string never appears as those characters
// anywhere. The one that got fixed first was fixed by hand, and the second
// encoding was missed, twice.
//
// So this audit decodes rather than searches, and holds the buttons to the
// promise the prose makes about them: that every button registers the same
// configuration. Buttons are grouped by the command they launch, since a
// Docker button and an npx button are different configurations on purpose, and
// within a group the arguments have to agree.
package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// buttonConfig is the part of a client's MCP entry the buttons carry. Fields
// this audit does not compare are deliberately absent: an unknown key must not
// make a payload unparseable, and json.Unmarshal ignores what it is not given.
type buttonConfig struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
}

// button is one decoded payload, kept with where it came from so a failure
// names a file and a line rather than a hash.
type button struct {
	File    string
	Line    int
	Host    string
	Config  buttonConfig
	Decoded string
}

// configParam matches the payload of an install URL. The "amp;" alternative is
// there because these links live in HTML blocks inside Markdown, where the
// ampersand is escaped.
//
// The payload runs to the end of the parameter rather than over a fixed
// character set: percent-encoded JSON keeps dots and colons literal, and a set
// that forgot one of them would truncate the payload into something that fails
// to parse for a reason that has nothing to do with the button.
var configParam = regexp.MustCompile(`https://([a-zA-Z0-9.-]+)/[^"'\s()\[\]]*?[?&](?:amp;)?config=([^"'\s&()\[\]]{40,})`)

// scanRoots are the files a reader can click a button from. Generated output
// under site/dist is excluded: it is a copy, and fixing it means fixing this.
var scanRoots = []string{"README.md", "docs", "site/src", ".vscode", "mcpb"}

// skipDirs are generated or vendored trees whose buttons are copies of the
// ones this audit already checks at their source.
var skipDirs = map[string]bool{
	"node_modules": true, "dist": true, ".astro": true,
}

var scanExtensions = map[string]bool{
	".md": true, ".mdx": true, ".json": true, ".html": true, ".astro": true,
}

func main() {
	dir := flag.String("dir", ".", "repository root to audit")
	verbose := flag.Bool("v", false, "list every button that was checked")
	flag.Parse()

	os.Exit(run(*dir, *verbose, os.Stdout, os.Stderr))
}

// run is main with its streams and its exit status handed to it, so the three
// ways this audit ends are reachable from a test instead of only from a
// process. It returns the exit status rather than calling os.Exit.
func run(dir string, verbose bool, out, errOut io.Writer) int {
	buttons, err := collect(dir)
	if err != nil {
		fmt.Fprintln(errOut, "audit_install_buttons:", err)
		return 1
	}
	if len(buttons) == 0 {
		fmt.Fprintln(errOut, "audit_install_buttons: no install buttons found, which means this audit is looking in the wrong place")
		return 1
	}

	if verbose {
		for _, b := range buttons {
			fmt.Fprintf(out, "%s:%d (%s) %s\n", b.File, b.Line, b.Host, b.Decoded)
		}
	}

	problems := check(buttons)
	if len(problems) > 0 {
		for _, p := range problems {
			fmt.Fprintln(errOut, p)
		}
		fmt.Fprintf(errOut, "\naudit_install_buttons: %d problem(s) across %d button(s)\n", len(problems), len(buttons))
		return 1
	}
	fmt.Fprintf(out, "audit_install_buttons: %d buttons decode cleanly and agree within each command\n", len(buttons))
	return 0
}

// collect finds and decodes every install button under dir.
func collect(dir string) ([]button, error) {
	var found []button
	for _, root := range scanRoots {
		buttons, err := collectRoot(dir, filepath.Join(dir, root))
		if err != nil {
			return nil, err
		}
		found = append(found, buttons...)
	}
	sort.Slice(found, func(i, j int) bool {
		if found[i].File != found[j].File {
			return found[i].File < found[j].File
		}
		return found[i].Line < found[j].Line
	})
	return found, nil
}

// collectRoot scans one entry of [scanRoots], which may be a single file or a
// directory to walk. A root that does not exist is not an error: the audit runs
// from the repository root and from a test's temporary directory, and the
// latter has only the files the test wrote.
func collectRoot(dir, path string) ([]button, error) {
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return scanFile(dir, path)
	}

	var found []button
	walkErr := filepath.WalkDir(path, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return fs.SkipDir
			}
			return nil
		}
		if !scanExtensions[strings.ToLower(filepath.Ext(p))] {
			return nil
		}
		buttons, scanErr := scanFile(dir, p)
		if scanErr != nil {
			return scanErr
		}
		found = append(found, buttons...)
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	return found, nil
}

func scanFile(root, path string) ([]button, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	text := string(data)
	rel, relErr := filepath.Rel(root, path)
	if relErr != nil {
		rel = path
	}

	var out []button
	for _, m := range configParam.FindAllStringSubmatchIndex(text, -1) {
		host := text[m[2]:m[3]]
		payload := text[m[4]:m[5]]
		decoded, decodeErr := decodePayload(payload)
		if decodeErr != nil {
			return nil, fmt.Errorf("%s:%d: the %s button's payload does not decode: %w",
				rel, lineOf(text, m[0]), host, decodeErr)
		}
		var cfg buttonConfig
		if unmarshalErr := json.Unmarshal([]byte(decoded), &cfg); unmarshalErr != nil {
			return nil, fmt.Errorf("%s:%d: the %s button decodes to something that is not a client entry: %w",
				rel, lineOf(text, m[0]), host, unmarshalErr)
		}
		out = append(out, button{
			File: filepath.ToSlash(rel), Line: lineOf(text, m[0]),
			Host: host, Config: cfg, Decoded: decoded,
		})
	}
	return out, nil
}

// decodePayload reads a payload through the encodings these links use, which
// are not one encoding: VS Code takes the JSON percent-encoded and nothing
// else, while Cursor and LM Studio take it base64 in either alphabet, with or
// without padding, and possibly percent-encoded on top of that. Assuming a
// single form is how the base64 copies of a retired flag survived a sweep that
// only knew about the percent-encoded ones.
func decodePayload(payload string) (string, error) {
	candidates := []string{payload}
	if unescaped, err := url.QueryUnescape(payload); err == nil && unescaped != payload {
		candidates = append(candidates, unescaped)
	}
	for _, candidate := range candidates {
		if strings.HasPrefix(strings.TrimSpace(candidate), "{") {
			return candidate, nil
		}
	}
	var lastErr error
	for _, candidate := range candidates {
		for _, enc := range []*base64.Encoding{
			base64.StdEncoding, base64.RawStdEncoding,
			base64.URLEncoding, base64.RawURLEncoding,
		} {
			decoded, err := enc.DecodeString(candidate)
			if err != nil {
				lastErr = err
				continue
			}
			return string(decoded), nil
		}
	}
	if lastErr == nil {
		lastErr = errors.New("no encoding matched")
	}
	return "", lastErr
}

func lineOf(text string, offset int) int {
	return strings.Count(text[:offset], "\n") + 1
}

// check holds every button that launches the same command to the same
// arguments, which is what the pages claim when they say the buttons register
// the configuration shown beside them.
func check(buttons []button) []string {
	byCommand := map[string][]button{}
	for _, b := range buttons {
		byCommand[b.Config.Command] = append(byCommand[b.Config.Command], b)
	}

	commands := make([]string, 0, len(byCommand))
	for command := range byCommand {
		commands = append(commands, command)
	}
	sort.Strings(commands)

	var problems []string
	for _, command := range commands {
		group := byCommand[command]
		reference := group[0]
		want := strings.Join(reference.Config.Args, " ")
		for _, b := range group[1:] {
			got := strings.Join(b.Config.Args, " ")
			if got == want {
				continue
			}
			problems = append(problems, fmt.Sprintf(
				"%s:%d: the %s button runs %q with different arguments than %s:%d\n    this button: %s\n    the others:  %s",
				b.File, b.Line, b.Host, command, reference.File, reference.Line, got, want,
			))
		}
	}
	return problems
}
