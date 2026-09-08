package service

import (
	"regexp"
	"strings"
)

var secretLikePatterns = []struct {
	name string
	re   *regexp.Regexp
}{
	{name: "private_key_pem", re: regexp.MustCompile(`-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----`)},
	{name: "github_pat", re: regexp.MustCompile(`\bghp_[A-Za-z0-9]{20,}\b`)},
	{name: "github_oauth", re: regexp.MustCompile(`\bgho_[A-Za-z0-9]{20,}\b`)},
	{name: "github_fine_grained", re: regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{20,}\b`)},
	{name: "aws_access_key", re: regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`)},
	{name: "slack_token", re: regexp.MustCompile(`\bxox[baprs]-[A-Za-z0-9-]{10,}\b`)},
	{name: "generic_bearer", re: regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._\-+=/]{20,}`)},
}

// SecretLikeFindings returns stable labels for secret-like substrings in text.
// This is a soft heuristic for warnings, not a DLP gate.
func SecretLikeFindings(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	out := make([]string, 0)
	seen := map[string]struct{}{}
	for _, pat := range secretLikePatterns {
		if pat.re.FindStringIndex(text) == nil {
			continue
		}
		if _, ok := seen[pat.name]; ok {
			continue
		}
		seen[pat.name] = struct{}{}
		out = append(out, pat.name)
	}
	return out
}
