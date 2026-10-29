package main

import "testing"

func TestUnformatted(t *testing.T) {
	tests := []struct{ name, in, want string }{
		// Whitespace
		{"collapses whitespace", "a\tb\r\n  c d", "a b c d"},
		{"only whitespace", " \n\t ", ""},

		// Words hyphenated at the end of a line are joined
		{"line-end hyphen", "he-\nllo", "hello"},
		{"windows line break", "he-\r\nllo", "hello"},
		{"spaces around the break", "he- \n   llo world", "hello world"},
		{"typographic hyphen", "he‐\nllo", "hello"},
		{"several in a passage", "The quick brown fox jum-\nped over the la-\nzy dog.", "The quick brown fox jumped over the lazy dog."},
		{"capitalised word", "Mex-\nico", "Mexico"},
		{"soft hyphens", "hel­lo wor­\nld", "hello world"},

		// Hyphens that belong to the word are kept, without the break
		{"proper name", "Jean-\nPaul", "Jean-Paul"},
		{"digit after", "COVID-\n19 and mid-\n1990s", "COVID-19 and mid-1990s"},
		{"digit before", "1990-\n1995", "1990-1995"},
		{"single letter before", "e-\nmail and X-\nray", "e-mail and X-ray"},
		{"compound, earlier hyphen", "mother-in-\nlaw", "mother-in-law"},
		{"compound, later hyphen", "mother-\nin-law", "mother-in-law"},
		{"all caps", "ANTI-\nWAR", "ANTI-WAR"},

		// Hanging hyphens keep their space
		{"hanging hyphen", "pre-\nand post-war", "pre- and post-war"},
		{"hanging hyphen, spanish", "corto-\ny largo plazo", "corto- y largo plazo"},

		// Left alone
		{"no line break", "he- llo", "he- llo"},
		{"paragraph break", "the end-\n\nNext paragraph", "the end- Next paragraph"},
		{"bullets", "- one\n- two", "- one - two"},
		{"spaced dash", "one -\ntwo", "one - two"},
		{"trailing hyphen", "trailing-", "trailing-"},
		{"en dash", "1990–\n1995", "1990– 1995"},

		// Known limit: a compound broken at its only hyphen reads like a syllable break
		{"compound broken at its hyphen", "well-\nknown", "wellknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := unformatted(tt.in); got != tt.want {
				t.Errorf("unformatted(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
