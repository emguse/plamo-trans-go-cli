package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runForTest(t *testing.T, args []string, input string) (int, string, string) {
	t.Helper()
	dir := t.TempDir()
	inPath := filepath.Join(dir, "input")
	os.WriteFile(inPath, []byte(input), 0600)
	in, err := os.Open(inPath)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	out, err := os.Create(filepath.Join(dir, "stdout"))
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	stderr, err := os.Create(filepath.Join(dir, "stderr"))
	if err != nil {
		t.Fatal(err)
	}
	defer stderr.Close()
	code := run(context.Background(), args, in, out, stderr)
	b, _ := os.ReadFile(out.Name())
	e, _ := os.ReadFile(stderr.Name())
	return code, string(b), string(e)
}
func TestCLIEndToEnd(t *testing.T) {
	var requests []map[string]any
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b map[string]any
		json.NewDecoder(r.Body).Decode(&b)
		requests = append(requests, b)
		if b["stream"] == true {
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"translated\"}}]}\n\ndata: [DONE]\n\n")
		} else {
			fmt.Fprint(w, `{"choices":[{"message":{"content":"translated"}}]}`)
		}
	}))
	defer s.Close()
	args := []string{"hello", "--server", s.URL, "-t", "en", "--glossary", "a=>b"}
	code, out, errs := runForTest(t, args, "")
	if code != 0 || out != "translated\n" || errs != "" {
		t.Fatal(code, out, errs)
	}
	doc := "# Title\n\n```go\n<|plamo:tag|>\n```\n\nLast\n"
	dir := t.TempDir()
	input := filepath.Join(dir, "in.md")
	output := filepath.Join(dir, "out.md")
	os.WriteFile(input, []byte(doc), 0600)
	code, out, errs = runForTest(t, []string{"--server", s.URL, "-f", input, "-o", output}, "")
	b, _ := os.ReadFile(output)
	if code != 0 || out != "" || string(b) != "translated\n\n```go\n<|plamo:tag|>\n```\n\ntranslated\n" {
		t.Fatal(code, out, errs, string(b))
	}
	code, out, errs = runForTest(t, []string{"--server", s.URL}, "first\n\nsecond\n")
	if code != 0 || out != "translated\n\ntranslated\n" {
		t.Fatal(code, out, errs)
	}
	code, out, errs = runForTest(t, []string{"--server", s.URL, "-i"}, "one\n\ntwo\n\n/clear\nthree\n\n/target ja\nfour\n\n/quit\n")
	if code != 0 || out != strings.Repeat("translated\n", 4) {
		t.Fatal(code, out, errs)
	}
	tail := requests[len(requests)-4:]
	for i, n := range []int{1, 3, 1, 1} {
		if len(tail[i]["messages"].([]any)) != n {
			t.Fatal("REPL history", tail)
		}
	}
	if tail[3]["chat_template_kwargs"].(map[string]any)["target_lang"] != "ja" {
		t.Fatal("target command", tail)
	}
	count := len(requests)
	for _, args := range [][]string{{"--server", s.URL, "<|plamo:tag|>"}, {"--server", s.URL, "-s", "ja", "text"}, {"--server", s.URL, "--context", "<|plamo:bos|>", "text"}} {
		code, _, _ = runForTest(t, args, "")
		if code != 1 {
			t.Fatal("unsafe input accepted", args)
		}
	}
	if len(requests) != count {
		t.Fatal("sent unsafe input")
	}
	code, _, _ = runForTest(t, []string{"--help"}, "")
	if code != 0 {
		t.Fatal("help")
	}
	code, _, _ = runForTest(t, []string{"--server", s.URL}, " \n")
	if code != 0 || len(requests) != count {
		t.Fatal("empty input should not call server")
	}
}

func TestServerOverridesBuiltInSize(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "unused-cache")
	t.Setenv("HF_HUB_CACHE", cache)
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.Model != modelAlias {
			t.Error("size changed the API alias", body.Model)
		}
		fmt.Fprint(w, `{"choices":[{"message":{"content":"translated"}}]}`)
	}))
	defer s.Close()
	code, out, errs := runForTest(t, []string{"--size", "8b", "--quant", "Q4_0", "--server", s.URL, "--offline", "hello"}, "")
	if code != 0 || out != "translated\n" {
		t.Fatal(code, out, errs)
	}
	if _, err := os.Stat(cache); !os.IsNotExist(err) {
		t.Fatal("external server touched model cache", err)
	}
}
