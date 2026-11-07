package main

import (
	"math"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Copies that look like a key, token or password are held back instead of sent. The checks are rough
// on purpose and err towards letting text through: a quote always has spaces in it, so only a lone
// string of letters and digits is ever judged on how random it looks.

const (
	minSecretLength  = 20
	maxSecretLength  = 200
	minSecretEntropy = 3.5 // Bits per character. English sits around 3, random keys above 4.
)

// Shapes of keys and tokens that can be recognised on sight. Each must match a whole word of the copy.
var secretShapes = []struct {
	what    string // Completes "Looks like …"
	pattern *regexp.Regexp
}{
	{"an API key", regexp.MustCompile(`^sk-[A-Za-z0-9_-]{20,}$`)}, // OpenAI, Anthropic and others
	{"a Stripe key", regexp.MustCompile(`^sk_(live|test)_[A-Za-z0-9]{10,}$`)},
	{"a GitHub token", regexp.MustCompile(`^(gh[opsur]_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{22,})$`)},
	{"a Slack token", regexp.MustCompile(`^xox[abp]-[A-Za-z0-9-]{10,}$`)},
	{"an AWS access key", regexp.MustCompile(`^(AKIA|ASIA)[0-9A-Z]{16}$`)}, // ASIA is the temporary variant
	{"a Google API key", regexp.MustCompile(`^AIza[0-9A-Za-z_-]{35}$`)},
	{"a GitLab token", regexp.MustCompile(`^glpat-[A-Za-z0-9_-]{20,}$`)},
	{"an npm token", regexp.MustCompile(`^npm_[A-Za-z0-9]{20,}$`)},
	{"a PyPI token", regexp.MustCompile(`^pypi-[A-Za-z0-9_-]{20,}$`)},
	{"an access token", regexp.MustCompile(`^eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+$`)}, // JWT
	{"a password in a URL", regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*://[^/\s:@]+:[^/\s@]+@`)},  // postgres://user:pass@host
}

var (
	privateKeyHeader = regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY[A-Z ]*-----`)
	bearerToken      = regexp.MustCompile(`^[A-Za-z0-9_.~+/=-]{20,}$`) // The word after "Bearer"
)

// Quotes and punctuation that often wrap a token when it's copied from code or JSON.
// Not "=" or "-": they're part of base64 and of many keys.
const wordPunctuation = "\"'`.,;:!?()[]{}<>"

// Reports whether the text looks like a secret and, if so, what kind, phrased to follow "Looks like".
// Runs on every new copy, so it returns at the first match and only measures entropy on a lone word.
func looksLikeSecret(text string) (bool, string) {
	text = strings.TrimSpace(text)
	if strings.Contains(text, "PRIVATE KEY") && privateKeyHeader.MatchString(text) {
		return true, "a private key"
	}

	fields := strings.Fields(text)
	for i, field := range fields {
		word := strings.Trim(field, wordPunctuation)
		if word == "Bearer" && i+1 < len(fields) && bearerToken.MatchString(strings.Trim(fields[i+1], wordPunctuation)) {
			return true, "an access token"
		}
		if len(word) < minSecretLength {
			continue // Every shape is at least this long, so most words are skipped without a regexp
		}
		for _, shape := range secretShapes {
			if shape.pattern.MatchString(word) {
				return true, shape.what
			}
		}
	}

	if len(fields) == 1 && isRandomString(strings.Trim(fields[0], wordPunctuation)) {
		return true, "a secret"
	}
	return false, ""
}

// A lone string of letters and digits with high entropy: a password, an opaque key, a hash.
// Addresses are left alone however random they look, since they're what people copy most.
func isRandomString(s string) bool {
	length := utf8.RuneCountInString(s)
	if length < minSecretLength || length > maxSecretLength {
		return false
	}
	if strings.Contains(s, "://") || strings.HasPrefix(s, "www.") || strings.Contains(s, "@") {
		return false // A URL or an e-mail address
	}

	var letters, digits bool
	counts := make(map[rune]int, length)
	for _, r := range s {
		letters = letters || unicode.IsLetter(r)
		digits = digits || unicode.IsDigit(r)
		counts[r]++
	}
	if !letters || !digits || isJoinedWords(s) {
		return false
	}

	// Shannon entropy
	entropy := 0.0
	for _, count := range counts {
		p := float64(count) / float64(length)
		entropy -= p * math.Log2(p)
	}
	return entropy >= minSecretEntropy
}

// Words joined by hyphens, underscores or dots: a slug, a file name, a version. Long ones have as much
// entropy as a key, but each part is a word, a number, or a word with a number attached ("v2", "IMG2024"),
// so letters and digits switch at most once. A key switches back and forth.
func isJoinedWords(s string) bool {
	for _, part := range strings.FieldsFunc(s, func(r rune) bool { return r == '-' || r == '_' || r == '.' }) {
		var switches int
		var lastDigit bool
		for i, r := range part {
			digit := unicode.IsDigit(r)
			if !digit && !unicode.IsLetter(r) {
				return false
			}
			if i > 0 && digit != lastDigit {
				switches++
			}
			lastDigit = digit
		}
		if switches > 1 {
			return false
		}
	}
	return true
}
