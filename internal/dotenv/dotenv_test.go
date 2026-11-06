package dotenv

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	legacyFixture := strings.Join([]string{
		"# A comment line at top",
		"BARE=abc123",
		`DOUBLE_QUOTED="def456"`,
		`SINGLE_QUOTED='ghi789'`,
		"EMPTY_BARE=",
		`EMPTY_DOUBLE=""`,
		`EMPTY_SINGLE=''`,
		`EMBEDDED_EQUALS="key=val=more"`,
		`EMBEDDED_SPACES="hello world"`,
		`LEADING_TRAILING_SPACES="  spaces  "`,
		`DOUBLE_INNER_SINGLE="it's quoted"`,
		`SINGLE_INNER_DOUBLE='he said "hi"'`,
		"   INDENTED_KEY=indented_value",
		"# Comment in the middle",
		`ASYMMETRIC_DOUBLE_LEFT="oops`,
		`ASYMMETRIC_DOUBLE_RIGHT=oops"`,
		`ASYMMETRIC_MIXED="foo'`,
		`NUMERIC_QUOTED="42"`,
		"",
	}, "\n")

	tests := []struct {
		name  string
		input string
		want  map[string]string
	}{
		{"bare value", "BARE=abc123", map[string]string{"BARE": "abc123"}},
		{"double-quoted", `K="def456"`, map[string]string{"K": "def456"}},
		{"single-quoted", `K='ghi789'`, map[string]string{"K": "ghi789"}},
		{"double-quoted numeric", `K="42"`, map[string]string{"K": "42"}},
		{"bare empty", "K=", map[string]string{"K": ""}},
		{"double-quoted empty", `K=""`, map[string]string{"K": ""}},
		{"single-quoted empty", `K=''`, map[string]string{"K": ""}},
		{"embedded equals in quotes", `K="key=val=more"`, map[string]string{"K": "key=val=more"}},
		{"embedded equals bare", "K=a=b=c", map[string]string{"K": "a=b=c"}},
		{"embedded spaces", `K="hello world"`, map[string]string{"K": "hello world"}},
		{"inner spaces kept inside quotes", `K="  spaces  "`, map[string]string{"K": "  spaces  "}},
		{"double with inner single", `K="it's quoted"`, map[string]string{"K": "it's quoted"}},
		{"single with inner double", `K='he said "hi"'`, map[string]string{"K": `he said "hi"`}},
		{"indented key", "   K=indented_value", map[string]string{"K": "indented_value"}},
		{"left-only double quote kept", `K="oops`, map[string]string{"K": `"oops`}},
		{"right-only double quote kept", `K=oops"`, map[string]string{"K": `oops"`}},
		{"asymmetric mixed quotes kept", `K="foo'`, map[string]string{"K": `"foo'`}},
		{"lone quote kept", `K="`, map[string]string{"K": `"`}},
		{"unquoted value trimmed", "K=  v \t", map[string]string{"K": "v"}},
		{"key trimmed around equals", "  K  =v", map[string]string{"K": "v"}},
		{"CRLF endings", "A=1\r\nB=\"2\"\r\n", map[string]string{"A": "1", "B": "2"}},
		{"lone CR endings", "A=1\rB=2\r", map[string]string{"A": "1", "B": "2"}},
		{"final line without newline", "A=1\nB=2", map[string]string{"A": "1", "B": "2"}},
		{"UTF-8 BOM", "\xef\xbb\xbfA=1\nB=2\n", map[string]string{"A": "1", "B": "2"}},
		{"export prefix", "export K=value", map[string]string{"K": "value"}},
		{"indented export prefix", "  export K=value", map[string]string{"K": "value"}},
		{"export as key name", "export=value", map[string]string{"export": "value"}},
		{"exportK is a plain key", "exportK=value", map[string]string{"exportK": "value"}},
		{"invalid key names skipped", "1BAD=x\nBAD-KEY=x\nBAD KEY=x\n=x\n.A=x\nGOOD_1=y\n_ok=z", map[string]string{"GOOD_1": "y", "_ok": "z"}},
		{"hash comment lines skipped", "# C=1\n   # D=2\nE=3", map[string]string{"E": "3"}},
		{"lines without equals skipped", "NOEQUALS\nA=1", map[string]string{"A": "1"}},
		{"blank lines skipped", "\n\n  \nA=1\n\n", map[string]string{"A": "1"}},
		{"no inline comment stripping", "K=value # note", map[string]string{"K": "value # note"}},
		{"no variable expansion", "A=1\nB=$A${A}", map[string]string{"A": "1", "B": "$A${A}"}},
		{"no unescaping inside quotes", `K="a\nb\"c"`, map[string]string{"K": `a\nb\"c`}},
		{"last duplicate wins", "K=1\nK=2", map[string]string{"K": "2"}},
		{"only one pair of quotes stripped", `K=""x""`, map[string]string{"K": `"x"`}},
		{"empty input", "", map[string]string{}},
		{"legacy fixture", legacyFixture, map[string]string{
			"BARE":                    "abc123",
			"DOUBLE_QUOTED":           "def456",
			"SINGLE_QUOTED":           "ghi789",
			"EMPTY_BARE":              "",
			"EMPTY_DOUBLE":            "",
			"EMPTY_SINGLE":            "",
			"EMBEDDED_EQUALS":         "key=val=more",
			"EMBEDDED_SPACES":         "hello world",
			"LEADING_TRAILING_SPACES": "  spaces  ",
			"DOUBLE_INNER_SINGLE":     "it's quoted",
			"SINGLE_INNER_DOUBLE":     `he said "hi"`,
			"INDENTED_KEY":            "indented_value",
			"ASYMMETRIC_DOUBLE_LEFT":  `"oops`,
			"ASYMMETRIC_DOUBLE_RIGHT": `oops"`,
			"ASYMMETRIC_MIXED":        `"foo'`,
			"NUMERIC_QUOTED":          "42",
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Parse(strings.NewReader(tc.input))
			if err != nil {
				t.Fatalf("Parse returned error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %d keys, want %d keys; differing keys: %s",
					len(got), len(tc.want), differingKeys(got, tc.want))
			}
		})
	}
}

func differingKeys(got, want map[string]string) string {
	var bad []string
	for k, w := range want {
		if g, ok := got[k]; !ok || g != w {
			bad = append(bad, k)
		}
	}
	for k := range got {
		if _, ok := want[k]; !ok {
			bad = append(bad, k)
		}
	}
	return strings.Join(bad, ",")
}

func TestLoadCurrentDirectoryMissingFile(t *testing.T) {
	t.Chdir(t.TempDir())
	got, err := LoadCurrentDirectory()
	if err != nil {
		t.Fatalf("missing .env must not be an error, got %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Errorf("want empty non-nil map, got %d keys (nil=%v)", len(got), got == nil)
	}
}

func TestLoadCurrentDirectoryReadsFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("A=1\nB=\"two\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	got, err := LoadCurrentDirectory()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"A": "1", "B": "two"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %d keys, want %d keys; differing keys: %s", len(got), len(want), differingKeys(got, want))
	}
}

func TestLoadCurrentDirectoryUnreadableIsError(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".env"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	if _, err := LoadCurrentDirectory(); err == nil {
		t.Fatal("reading a .env that is a directory must return an error")
	}
}
