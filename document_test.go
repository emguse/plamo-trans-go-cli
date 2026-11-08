package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTranslateReadmeWithReservedTokenExamples(t *testing.T) {
	// Exercise the actual README so documentation and input validation cannot drift.
	doc, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []message `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		for _, m := range body.Messages {
			if strings.Contains(m.Content, "<|plamo:") {
				t.Error("reserved token sent to model", m)
			}
		}
		calls++
		// Echo the source as SSE to verify the entire document roundtrips unchanged.
		b, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"content": body.Messages[len(body.Messages)-1].Content}}}})
		fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", b)
	}))
	defer s.Close()
	output := filepath.Join(t.TempDir(), "README.en.md")
	code, out, errs := runForTest(t, []string{"--server", s.URL, "-f", "README.md", "-o", output}, "")
	if code != 0 || out != "" {
		t.Fatal(code, out, errs)
	}
	translated, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if string(translated) != string(doc) {
		t.Fatal("README formatting or code examples changed")
	}
	if calls == 0 {
		t.Fatal("README was not translated")
	}
}
