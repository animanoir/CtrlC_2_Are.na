package main

import (
	"strings"
	"testing"
)

// The keys below are made up to have the right shape. Written out whole, they look real enough for GitHub's push
// protection to block the commit, so no complete token appears as one literal in this file: each is put together
// from pieces when the test runs. "want" is what the app says the text looks like, or empty when it should go through.
func TestLooksLikeSecret(t *testing.T) {
	tests := []struct{ name, in, want string }{
		// Keys and tokens with a known shape
		{"openai key", fake("sk-proj-", "Ab3dEf6hIj9kLm2nOp5q", "Rs8tUv1wXy4zAb3dEf6h"), "an API key"},
		{"anthropic key", fake("sk-ant-api03-", "Ab3dEf6hIj9kLm2nOp5qRs8tUv1wXy4z", "_Ab3dEf6hIj9kLm2nOp5qRs8tUv1wXy4zAA"), "an API key"},
		{"stripe live key", fake("sk_live_", "4eC39HqLyjWD", "arjtT1zdp7dc"), "a Stripe key"},
		{"stripe test key", fake("sk_test_", "4eC39HqLyjWD", "arjtT1zdp7dc"), "a Stripe key"},
		{"github personal token", fake("ghp_", "16C7e42F292c6912", "E7710c838347Ae178B4a"), "a GitHub token"},
		{"github oauth token", fake("gho_", "16C7e42F292c6912", "E7710c838347Ae178B4a"), "a GitHub token"},
		{"github fine-grained token", fake("github_pat_", "11ABCDEFG0abcdefghijkl", "_ABCDEFGHIJKLMNOPQRSTUVWXYZ", "abcdefghijklmnopqrstuvwxyz0123456"), "a GitHub token"},
		{"slack bot token", fake("xoxb-", "1234567890-1234567890123", "-AbCdEfGhIjKlMnOpQrStUvWx"), "a Slack token"},
		{"slack user token", fake("xoxp-", "1234567890-1234567890123-1234567890123", "-a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6"), "a Slack token"},
		{"aws access key", fake("AKIA", "IOSFODNN", "7EXAMPLE"), "an AWS access key"},
		{"aws temporary key", fake("ASIA", "IOSFODNN", "7EXAMPLE"), "an AWS access key"},
		{"google api key", fake("AIza", "SyD-9tSrke72PouQMnMX", "-a7eZSW0jkFMBWY"), "a Google API key"},
		{"gitlab token", fake("glpat-", "xY2zAbCdEf", "GhIjKlMnOp"), "a GitLab token"},
		{"npm token", fake("npm_", "1a2B3c4D5e6F7g8H9i0J", "kLmNoPqRsTuVwXyZ"), "an npm token"},
		{"pypi token", fake("pypi-", "AgEIcHlwaS5vcmcCJGE4ZjU5YjY2", "LTM5NTEtNDM5Mi1hNzBjLWRkYjRjYjJhZTg2ZQ"), "a PyPI token"},
		{"jwt", fake("eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9", ".eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIn0", ".SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"), "an access token"},
		{"bearer header", fake("Authorization: Bearer ", "7f3a9c2e4b8d1f6a", "5e0c9b2d4f7a1e3c"), "an access token"},
		{"rsa private key", fake("-----BEGIN RSA ", "PRIVATE KEY-----\n", "MIIEowIBAAKCAQEA0Z3VS5JJcds3xfn", "\n-----END RSA ", "PRIVATE KEY-----"), "a private key"},
		{"openssh private key", fake("-----BEGIN OPENSSH ", "PRIVATE KEY-----\n", "b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQ", "AAAAAAAAABAAAAMwAAAAtzc2gtZW", "\n-----END OPENSSH ", "PRIVATE KEY-----"), "a private key"},
		{"unlabelled private key", fake("-----BEGIN ", "PRIVATE KEY-----\n", "MIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQC7", "\n-----END ", "PRIVATE KEY-----"), "a private key"},
		{"database url with password", fake("postgres://admin:", "S3cretPassw0rd", "@db.example.com:5432/app"), "a password in a URL"},

		// The shape is found inside other text too
		{"key in a sentence", fake("Here's the deploy key: ", "ghp_", "16C7e42F292c6912", "E7710c838347Ae178B4a", ", don't share it"), "a GitHub token"},
		{"key in json", fake(`{"token": "`, "xoxb-", "1234567890-1234567890123", "-AbCdEfGhIjKlMnOpQrStUvWx", `"}`), "a Slack token"},
		{"key on its own line", fake("export AWS_ACCESS_KEY_ID\n", "AKIA", "IOSFODNN", "7EXAMPLE", "\n"), "an AWS access key"},
		{"key in an env file", fake("OPENAI_API_KEY=", "sk-proj-", "Ab3dEf6hIj9kLm2nOp5q", "Rs8tUv1wXy4zAb3dEf6h"), "a secret"}, // The "=" hides the shape, but the whole line is still one random word
		{"aws prefix with wrong length", fake("AKIA", "IOSFODNN", "7EXAMPLES"), "a secret"},                                     // Not the exact AWS shape, but still a lone random word

		// A lone random-looking string
		{"random password", fake("xK9mP2vL8qR4tW7y", "B3nZ6cF1hJ5dG0sA"), "a secret"},
		{"hex token", fake("3f9a7c2e5b8d1f4a6e0c9b3d7f2a5e8c", "1b4d6f9a3c7e0b2d5f8a1c4e7b0d3f6a"), "a secret"},
		{"base64 secret", fake("wJalrXUtnFEMI/K7MDENG", "/bPxRfiCYEXAMPLEKEY"), "a secret"},
		{"quoted secret", fake(`"`, "xK9mP2vL8qR4tW7y", "B3nZ6cF1hJ5dG0sA", `"`), "a secret"},
		{"surrounding whitespace", fake("  ", "xK9mP2vL8qR4tW7y", "B3nZ6cF1hJ5dG0sA", "\n"), "a secret"},

		// Left alone
		{"empty", "", ""},
		{"only whitespace", " \n\t ", ""},
		{"short word", "hello", ""},
		{"prose", "The quick brown fox jumps over the lazy dog, as it has in every typing test since 1885, when the sentence first appeared in print.", ""},
		{"quote with numbers", "In 1984, Apple sold 3,000,000 units at $2,495 each; by 1990 the figure had reached 10x that.", ""},
		{"url", "https://github.com/animanoir/CtrlC_2_Are.na/releases/latest", ""},
		{"url without scheme", "www.are.na/animanoir/reading-notes-2024", ""},
		{"email address", "firstname.lastname1984@example.com", ""},
		{"hex color", "#1a2b3c", ""},
		{"uuid in a sentence", "The order 123e4567-e89b-12d3-a456-426614174000 shipped yesterday.", ""},
		{"base64 with spaces", "QWxhZGRpbjpvcGVuIHNlc2FtZQ== QWxhZGRpbjpvcGVuIHNlc2FtZQ==", ""},
		{"timestamp", "2024-01-15T10:30:00Z", ""},
		{"long number", "1234567890123456789012", ""},
		{"long word", "Donaudampfschifffahrtsgesellschaftskapitän", ""},
		{"bearer in prose", "The bearer of bad news is rarely thanked. Bearer bonds are another matter.", ""},
		{"prefix with too little after it", "sk-123 and ghp_abc are placeholders", ""},
		{"slug", "desk-lamp-with-adjustable-arm-2024", ""},
		{"channel slug", "my-reading-notes-for-the-year-2024", ""},
		{"file name", "IMG_20240101_123456.jpg", ""},
		{"version", "release-notes-v2.0.1.md", ""},
		{"identifier with a digit", "clipboardMonitoringGoroutine2", ""},

		// Known limits: secrets that pass through on purpose, so that quotes never get held back
		{"password of only letters", "correcthorsebatterystaple", ""},
		{"short random token", "aB3dE6fG9h", ""},
		{"key inside a url", fake("https://api.example.com/v1/items?api_key=", "sk-", "Ab3dEf6hIj9kLm2nOp5q", "Rs8tUv1wXy4zAb3d"), ""},
		{"password in a sentence", fake("the wifi password is ", "xK9mP2vL8qR4tW7y", "B3nZ6cF1hJ5dG0sA"), ""},

		// Known limits: harmless strings held back because they can't be told from a key
		{"git commit hash", fake("3f2a9c7e1b8d4f6a0c2e", "5b9d7f1a3c8e6b0d2f4a"), "a secret"},
		{"bare uuid", "123e4567-e89b-12d3-a456-426614174000", "a secret"},
		{"name with a digit inside", "ctrl2arena-macos-universal-v2.0.1", "a secret"}, // "ctrl2arena" switches between letters and digits twice, like a key does
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, what := looksLikeSecret(tt.in)
			if got != (tt.want != "") || what != tt.want {
				t.Errorf("looksLikeSecret(%q) = %v, %q, want %v, %q", tt.in, got, what, tt.want != "", tt.want)
			}
		})
	}
}

// Joins the pieces of a fixture, so that no key appears whole in the source
func fake(pieces ...string) string {
	return strings.Join(pieces, "")
}
