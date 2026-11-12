package cli

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

const minimumCheckedReadmeLines = 5

func TestReadmeExamplesParse(t *testing.T) {
	data, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	lines := readmeCommandLines(string(data))
	if len(lines) < minimumCheckedReadmeLines {
		t.Fatalf("found %d README command lines, want at least %d", len(lines), minimumCheckedReadmeLines)
	}
	for _, l := range lines {
		if err := parseOnly(l.text); err != nil {
			t.Errorf("README.md line %d: %q: %v", l.number, l.text, err)
		}
	}
	t.Logf("checked %d README command lines", len(lines))
}

func TestParseOnlyRejectsUnknownCommandsAndFlags(t *testing.T) {
	for _, line := range []string{
		"cognitive3d-cli-upload sceen upload",
		"cognitive3d-cli-upload scene upload --no-such-flag x",
		"cognitive3d-cli-upload scene frobnicate",
		"cognitive3d-cli-upload scene upload 'unterminated",
	} {
		if err := parseOnly(line); err == nil {
			t.Errorf("%q parsed without error", line)
		}
	}
	valid := `cognitive3d-cli-upload scene upload --scene_dir "a b" --scene-name 'My Scene' --dry_run`
	if err := parseOnly(valid); err != nil {
		t.Errorf("valid line rejected: %v", err)
	}
}

type readmeLine struct {
	number int
	text   string
}

func readmeCommandLines(markdown string) []readmeLine {
	var found []readmeLine
	inFence := false
	var pending *readmeLine
	for i, raw := range strings.Split(markdown, "\n") {
		line := strings.TrimRight(raw, "\r")
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
			pending = nil
			continue
		}
		if !inFence {
			continue
		}
		trimmed := strings.TrimSpace(line)
		if pending != nil {
			pending.text += " " + strings.TrimSuffix(trimmed, "\\")
			if !strings.HasSuffix(trimmed, "\\") {
				found = append(found, *pending)
				pending = nil
			}
			continue
		}
		trimmed = strings.TrimPrefix(trimmed, "$ ")
		if !strings.HasPrefix(trimmed, binaryName+" ") {
			continue
		}
		entry := readmeLine{number: i + 1, text: strings.TrimSuffix(trimmed, "\\")}
		if strings.HasSuffix(trimmed, "\\") {
			pending = &entry
			continue
		}
		found = append(found, entry)
	}
	return found
}

func parseOnly(line string) error {
	words, err := splitShellWords(line)
	if err != nil {
		return err
	}
	args := words[1:]
	streams := testIO()
	root := newRootCmd(streams, &runtime{streams: streams})
	root.InitDefaultHelpCmd()
	root.InitDefaultCompletionCmd()
	cmd, remaining, err := root.Find(args)
	if err != nil {
		return err
	}
	if cmd == root && len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		return fmt.Errorf("unknown command %q", args[0])
	}
	if err := cmd.ParseFlags(remaining); err != nil {
		return err
	}
	positional := cmd.Flags().Args()
	if cmd.HasSubCommands() && len(positional) > 0 {
		return fmt.Errorf("unknown command %q for %q", positional[0], cmd.CommandPath())
	}
	return cmd.ValidateArgs(positional)
}

// splitShellWords splits on unquoted whitespace, honouring single quotes,
// double quotes and backslash escapes outside single quotes.
func splitShellWords(line string) ([]string, error) {
	var words []string
	var current strings.Builder
	inWord := false
	var quote rune
	escaped := false
	for _, r := range line {
		switch {
		case escaped:
			current.WriteRune(r)
			escaped = false
		case quote == '\'':
			if r == '\'' {
				quote = 0
			} else {
				current.WriteRune(r)
			}
		case r == '\\':
			escaped, inWord = true, true
		case quote == '"':
			if r == '"' {
				quote = 0
			} else {
				current.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote, inWord = r, true
		case r == ' ' || r == '\t':
			if inWord {
				words = append(words, current.String())
				current.Reset()
				inWord = false
			}
		default:
			current.WriteRune(r)
			inWord = true
		}
	}
	if quote != 0 || escaped {
		return nil, fmt.Errorf("unterminated quote or escape")
	}
	if inWord {
		words = append(words, current.String())
	}
	if len(words) == 0 || words[0] != binaryName {
		return nil, fmt.Errorf("not a %s command", binaryName)
	}
	return words, nil
}
