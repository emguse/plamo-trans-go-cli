package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestReadBlock(t *testing.T) {
	lines := []string{"a", "b", "", "/clear", "last"}
	next := func(_ string) (string, error) {
		if len(lines) == 0 {
			return "", io.EOF
		}
		s := lines[0]
		lines = lines[1:]
		return s, nil
	}
	for _, want := range []string{"a\nb", "/clear", "last"} {
		got, err := readBlock(next)
		if err != nil || got != want {
			t.Fatal(got, err)
		}
	}
	if _, err := readBlock(next); err != io.EOF {
		t.Fatal(err)
	}
	if _, err := readBlock(func(string) (string, error) { return strings.Repeat("あ", 30001), nil }); err == nil {
		t.Fatal("no block cap")
	}
}

func TestHistoryFileLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history")
	var b strings.Builder
	for i := 0; i < 1100; i++ {
		fmt.Fprintln(&b, i)
	}
	os.WriteFile(path, []byte(b.String()), 0600)
	compactHistory(path)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if len(lines) != 1000 || lines[0] != "100" || lines[999] != "1099" {
		t.Fatal("history not bounded", len(lines))
	}
}

func TestREPLTranslateCodeHistory(t *testing.T) {
	var requests [][]message
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []message `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		requests = append(requests, body.Messages)
		source := body.Messages[len(body.Messages)-1].Content
		if source == "fail" {
			http.Error(w, "failed", http.StatusInternalServerError)
			return
		}
		b, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"content": "translated " + source}}}})
		fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", b)
	}))
	defer server.Close()
	input := "first\n\n```text\nsecond\n```\n\nfail\n\nthird\n\n/clear\nfourth\n\n/target en\nfifth\n\n/quit\n"
	code, _, errs := runForTest(t, []string{"--server", server.URL, "--translate-code", "-i"}, input)
	if code != 0 || len(requests) != 6 {
		t.Fatal(code, errs, requests)
	}
	expected := [][]message{
		{{"user", "first"}},
		{{"user", "first"}, {"assistant", "translated first"}, {"user", "second"}},
		{{"user", "first"}, {"assistant", "translated first"}, {"user", "second"}, {"assistant", "translated second"}, {"user", "fail"}},
		{{"user", "first"}, {"assistant", "translated first"}, {"user", "second"}, {"assistant", "translated second"}, {"user", "third"}},
		{{"user", "fourth"}},
		{{"user", "fifth"}},
	}
	if !reflect.DeepEqual(requests, expected) {
		t.Fatalf("requests = %#v; want %#v", requests, expected)
	}
}
