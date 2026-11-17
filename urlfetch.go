package main

import (
	"context"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const urlFetchTimeout = 30 * time.Second

// parseInputURL validates an http(s) URL and rewrites GitHub blob URLs to raw content URLs.
func parseInputURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("--url must be an http(s) URL: %q", raw)
	}
	return githubRawURL(u), nil
}

// githubRawURL maps https://github.com/o/r/blob/ref/path to raw.githubusercontent.com.
func githubRawURL(u *url.URL) *url.URL {
	if u.Hostname() != "github.com" && u.Hostname() != "www.github.com" {
		return u
	}
	parts := strings.SplitN(strings.TrimPrefix(u.Path, "/"), "/", 5)
	if len(parts) < 5 || parts[2] != "blob" || parts[0] == "" || parts[1] == "" || parts[3] == "" || parts[4] == "" {
		return u
	}
	return &url.URL{Scheme: "https", Host: "raw.githubusercontent.com", Path: "/" + parts[0] + "/" + parts[1] + "/" + parts[3] + "/" + parts[4]}
}

func textContentType(ct string) error {
	if ct == "" {
		return nil // unknown; readLimited still requires valid UTF-8
	}
	mt, _, err := mime.ParseMediaType(ct)
	if err != nil {
		return fmt.Errorf("--url: invalid Content-Type %q", ct)
	}
	switch {
	case mt == "text/html" || mt == "application/xhtml+xml":
		return fmt.Errorf("--url: HTML pages are not supported (got %s); use a raw text/Markdown URL, or convert the page and pipe it via stdin", mt)
	case strings.HasPrefix(mt, "text/"), mt == "application/json", strings.HasSuffix(mt, "+json"),
		mt == "application/yaml", mt == "application/x-yaml", mt == "application/toml":
		return nil
	}
	return fmt.Errorf("--url: unsupported Content-Type %s (text only)", mt)
}

func fetchURL(ctx context.Context, raw string) (string, error) {
	u, err := parseInputURL(raw)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, urlFetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "text/markdown, text/plain, text/*;q=0.9, */*;q=0.1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("--url: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", fmt.Errorf("--url: %s returned %s", u.Redacted(), resp.Status)
	}
	if err := textContentType(resp.Header.Get("Content-Type")); err != nil {
		return "", err
	}
	return readLimited(resp.Body)
}
