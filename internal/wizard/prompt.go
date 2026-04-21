package wizard

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

const (
	errReadInput     = "reading input: %w"
	promptDefaultFmt = "%s [%s]: "
)

// Prompter handles interactive user input via a reader/writer pair.
type Prompter struct {
	reader *bufio.Reader
	writer io.Writer
}

// NewPrompter creates a Prompter reading from r and writing to w.
func NewPrompter(r io.Reader, w io.Writer) *Prompter {
	return &Prompter{reader: bufio.NewReader(r), writer: w}
}

// AskString prompts for a non-empty string. Repeats until non-empty input.
func (p *Prompter) AskString(prompt string) (string, error) {
	for {
		fmt.Fprintf(p.writer, "%s: ", prompt)
		line, err := p.reader.ReadString('\n')
		if err != nil {
			return "", fmt.Errorf(errReadInput, err)
		}
		val := strings.TrimSpace(line)
		if val != "" {
			return val, nil
		}
		fmt.Fprintln(p.writer, "  Value cannot be empty. Please try again.")
	}
}

// AskStringDefault prompts with a default value shown in brackets.
// Returns the default if the user presses Enter without typing.
func (p *Prompter) AskStringDefault(prompt, defaultVal string) (string, error) {
	fmt.Fprintf(p.writer, promptDefaultFmt, prompt, defaultVal)
	line, err := p.reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf(errReadInput, err)
	}
	val := strings.TrimSpace(line)
	if val == "" {
		return defaultVal, nil
	}
	return val, nil
}

// AskPassword prompts for sensitive input. The value is visible in the
// terminal since we use no TUI library; a warning is shown by the caller.
func (p *Prompter) AskPassword(prompt string) (string, error) {
	for {
		fmt.Fprintf(p.writer, "%s: ", prompt)
		line, err := p.reader.ReadString('\n')
		if err != nil {
			return "", fmt.Errorf(errReadInput, err)
		}
		val := strings.TrimSpace(line)
		if val != "" {
			return val, nil
		}
		fmt.Fprintln(p.writer, "  Value cannot be empty. Please try again.")
	}
}

// AskPasswordDefault prompts for sensitive input with a masked default.
// If the user presses Enter without typing, the existing value is kept.
func (p *Prompter) AskPasswordDefault(prompt, defaultVal string) (string, error) {
	masked := MaskToken(defaultVal)
	fmt.Fprintf(p.writer, promptDefaultFmt, prompt, masked)
	line, err := p.reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf(errReadInput, err)
	}
	val := strings.TrimSpace(line)
	if val == "" {
		return defaultVal, nil
	}
	return val, nil
}

// AskYesNo prompts for y/n. defaultYes determines the default for empty input.
func (p *Prompter) AskYesNo(prompt string, defaultYes bool) (bool, error) {
	hint := "y/N"
	if defaultYes {
		hint = "Y/n"
	}
	fmt.Fprintf(p.writer, promptDefaultFmt, prompt, hint)

	line, err := p.reader.ReadString('\n')
	if err != nil {
		return false, fmt.Errorf(errReadInput, err)
	}
	val := strings.TrimSpace(strings.ToLower(line))
	switch val {
	case "":
		return defaultYes, nil
	case "y", "yes":
		return true, nil
	case "n", "no":
		return false, nil
	default:
		return defaultYes, nil
	}
}

// AskChoice presents numbered options and returns the 0-based selected index.
func (p *Prompter) AskChoice(prompt string, options []string) (int, error) {
	for i, opt := range options {
		fmt.Fprintf(p.writer, "  [%d] %s\n", i+1, opt)
	}
	for {
		fmt.Fprintf(p.writer, "%s [1-%d]: ", prompt, len(options))
		line, err := p.reader.ReadString('\n')
		if err != nil {
			return 0, fmt.Errorf(errReadInput, err)
		}
		val := strings.TrimSpace(line)
		n, err := strconv.Atoi(val)
		if err != nil || n < 1 || n > len(options) {
			fmt.Fprintf(p.writer, "  Please enter a number between 1 and %d.\n", len(options))
			continue
		}
		return n - 1, nil
	}
}

// AskMultiChoice presents checkboxes for multi-select. Returns a boolean
// slice indicating which options were selected. Accepts space-separated
// numbers or "a" for all.
func (p *Prompter) AskMultiChoice(prompt string, options []string, defaults []bool) ([]bool, error) {
	for i, opt := range options {
		marker := " "
		if defaults != nil && i < len(defaults) && defaults[i] {
			marker = "*"
		}
		fmt.Fprintf(p.writer, "  [%d] %s %s\n", i+1, marker, opt)
	}

	for {
		fmt.Fprintf(p.writer, "%s [1-%d, a=all]: ", prompt, len(options))
		line, err := p.reader.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf(errReadInput, err)
		}
		val := strings.TrimSpace(strings.ToLower(line))

		selected := make([]bool, len(options))

		if val == "a" || val == "all" {
			for i := range selected {
				selected[i] = true
			}
			return selected, nil
		}

		if val == "" && defaults != nil {
			return defaults, nil
		}

		parts := strings.Fields(val)
		valid := true
		var n int
		for _, part := range parts {
			n, err = strconv.Atoi(part)
			if err != nil || n < 1 || n > len(options) {
				valid = false
				break
			}
			selected[n-1] = true
		}
		if !valid || len(parts) == 0 {
			fmt.Fprintf(p.writer, "  Enter space-separated numbers (1-%d) or 'a' for all.\n", len(options))
			continue
		}
		return selected, nil
	}
}
