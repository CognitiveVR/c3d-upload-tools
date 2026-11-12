package ui

import "testing"

func TestPrintableReplacesControlCharacters(t *testing.T) {
	cases := map[string]string{
		"plain name é 漢":     "plain name é 漢",
		"a\tb":               "a�b",
		"\x1b[31mred":        "�[31mred",
		"x\ny\r":             "x�y�",
		"p q ":               "p�q�",
		"\u009bcsi\x7f":      "�csi�",
		"":                   "",
		"a\u202ab\u202ec":    "a\ufffdb\ufffdc",
		"\u2066x\u2069":      "\ufffdx\ufffd",
		"\u2067\u2068":       "\ufffd\ufffd",
		"\u202b\u202c\u202d": "\ufffd\ufffd\ufffd",
		"\u2065\u206a\u2029": "\u2065\u206a\ufffd",
	}
	for in, want := range cases {
		if got := Printable(in); got != want {
			t.Errorf("Printable(%q) = %q, want %q", in, got, want)
		}
	}
}
