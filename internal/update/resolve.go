package update

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
)

type githubRelease struct {
	TagName string `json:"tag_name"`
}

func resolveTargetTag(ctx context.Context, opts Options) (string, error) {
	if opts.TargetVersion != "" {
		tag := opts.TargetVersion
		if !strings.HasPrefix(tag, "v") && looksLikeSemver(tag) {
			tag = "v" + tag
		}
		if err := validateTag(tag); err != nil {
			return "", err
		}
		return tag, nil
	}
	if opts.BaseURL != "" {
		return "", apperr.New(apperr.CodeInvalidInput, "explicit --version is required when RELEASE_BASE_URL is set")
	}
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", opts.Repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "atlas-tasker-update")
	resp, err := opts.HTTPClient.Do(req)
	if err != nil {
		return "", apperr.Wrap(apperr.CodeInternal, err, "fetch latest release metadata")
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", apperr.Wrap(apperr.CodeInternal, err, "read latest release metadata")
	}
	if resp.StatusCode != http.StatusOK {
		return "", apperr.New(apperr.CodeInternal, fmt.Sprintf("latest release lookup failed: HTTP %d", resp.StatusCode))
	}
	var release githubRelease
	if err := json.Unmarshal(body, &release); err != nil {
		return "", apperr.Wrap(apperr.CodeInternal, err, "parse latest release metadata")
	}
	tag := strings.TrimSpace(release.TagName)
	if err := validateTag(tag); err != nil {
		return "", err
	}
	return tag, nil
}

func validateTag(tag string) error {
	if tag == "" {
		return apperr.New(apperr.CodeInvalidInput, "release tag is empty")
	}
	for _, r := range tag {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' || r == '+' {
			continue
		}
		return apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("unsafe release version: %s", tag))
	}
	return nil
}

func looksLikeSemver(raw string) bool {
	if raw == "" {
		return false
	}
	for _, r := range raw {
		if (r >= '0' && r <= '9') || r == '.' || r == '-' || r == '+' {
			continue
		}
		return false
	}
	return true
}

func validateBaseURL(raw string, allowInsecure bool) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return apperr.Wrap(apperr.CodeInvalidInput, err, "parse RELEASE_BASE_URL")
	}
	switch parsed.Scheme {
	case "https":
		return nil
	case "http":
		host := parsed.Hostname()
		if allowInsecure && (host == "127.0.0.1" || host == "localhost" || host == "::1") {
			return nil
		}
		return apperr.New(apperr.CodeInvalidInput, "RELEASE_BASE_URL must use https://; loopback http requires ALLOW_INSECURE_RELEASE_BASE_URL=1")
	default:
		return apperr.New(apperr.CodeInvalidInput, "RELEASE_BASE_URL must use https://")
	}
}
