package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGithubRawURL(t *testing.T) {
	for in, want := range map[string]string{
		"https://github.com/o/r/blob/main/docs/a.md":      "https://raw.githubusercontent.com/o/r/main/docs/a.md",
		"https://github.com/o/r/blob/main/a.md?plain=1":   "https://raw.githubusercontent.com/o/r/main/a.md",
		"https://github.com/o/r":                          "https://github.com/o/r",
		"https://github.com/o/r/tree/main/docs":           "https://github.com/o/r/tree/main/docs",
		"https://example.com/o/r/blob/main/a.md":          "https://example.com/o/r/blob/main/a.md",
		"https://raw.githubusercontent.com/o/r/main/a.md": "https://raw.githubusercontent.com/o/r/main/a.md",
	} {
		u, _ := url.Parse(in)
		if got := githubRawURL(u); got.String() != want {
			t.Fatal(in, got, want)
		}
	}
}

func TestFetchURL(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/md":
			w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
			fmt.Fprint(w, "# Hi\r\n\r\nbody\r\n")
		case "/plain":
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprint(w, "plain")
		case "/html":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprint(w, "<html></html>")
		case "/bin":
			w.Header().Set("Content-Type", "application/octet-stream")
			fmt.Fprint(w, "x")
		case "/redir":
			http.Redirect(w, r, "/md", http.StatusFound)
		case "/badredir":
			http.Redirect(w, r, "file:///etc/passwd", http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	}))
	defer s.Close()
	ctx := context.Background()
	for path, want := range map[string]string{"/md": "# Hi\n\nbody\n", "/plain": "plain", "/redir": "# Hi\n\nbody\n"} {
		got, err := fetchURL(ctx, s.URL+path)
		if err != nil || got != want {
			t.Fatal(path, got, err)
		}
	}
	for path, msg := range map[string]string{"/html": "HTML pages are not supported", "/bin": "unsupported Content-Type", "/missing": "404", "/badredir": "--url"} {
		if _, err := fetchURL(ctx, s.URL+path); err == nil || !strings.Contains(err.Error(), msg) {
			t.Fatal(path, err)
		}
	}
	for _, bad := range []string{"file:///etc/passwd", "ftp://x/y", "not a url", "/relative"} {
		if _, err := fetchURL(ctx, bad); err == nil {
			t.Fatal("accepted", bad)
		}
	}
}

func TestURLArgumentsAndCLI(t *testing.T) {
	for _, args := range [][]string{{"-u", "https://x/a.md", "-f", "a.md"}, {"-u", "https://x/a.md", "-i"}, {"-u", "https://x/a.md", "text"}, {"-u", "ftp://x/a.md"}} {
		if _, err := parseArgs(args, os.Stderr); err == nil {
			t.Fatal("accepted", args)
		}
	}
	a, err := parseArgs([]string{"--url", "https://x/a.md", "-t", "en"}, os.Stderr)
	if err != nil || a.url != "https://x/a.md" {
		t.Fatal(a.url, err)
	}
	var requests int
	tr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b map[string]any
		json.NewDecoder(r.Body).Decode(&b)
		requests++
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"translated\"}}]}\n\ndata: [DONE]\n\n")
	}))
	defer tr.Close()
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/doc.md" {
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprint(w, "# Title\n\n```go\ncode\n```\n\nLast\n")
			return
		}
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, "<html></html>")
	}))
	defer src.Close()
	output := filepath.Join(t.TempDir(), "out.md")
	code, out, errs := runForTest(t, []string{"--server", tr.URL, "-u", src.URL + "/doc.md", "-o", output}, "")
	b, _ := os.ReadFile(output)
	if code != 0 || out != "" || string(b) != "translated\n\n```go\ncode\n```\n\ntranslated\n" {
		t.Fatal(code, out, errs, string(b))
	}
	requests = 0
	code, _, errs = runForTest(t, []string{"--server", tr.URL, "-u", src.URL + "/page"}, "")
	if code != 1 || requests != 0 || !strings.Contains(errs, "HTML pages are not supported") {
		t.Fatal(code, requests, errs)
	}
}
