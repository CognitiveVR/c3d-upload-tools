package ui

import "testing"

func TestPrintableReplacesControlCharacters(t *testing.T) {
	cases := map[string]string{
		"plain name é 漢": "plain name é 漢",
		"a\tb":           "a�b",
		"\x1b[31mred":    "�[31mred",
		"x\ny\r":         "x�y�",
		"p q ":           "p�q�",
		"\u009bcsi\x7f":  "�csi�",
		"":               "",
	}
	for in, want := range cases {
		if got := Printable(in); got != want {
			t.Errorf("Printable(%q) = %q, want %q", in, got, want)
		}
	}
}
