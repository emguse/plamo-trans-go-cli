package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestTranslationOptions(t *testing.T) {
	var inputs []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []message `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		input := body.Messages[len(body.Messages)-1].Content
		inputs = append(inputs, input)
		b, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"content": "訳\"\n" + input}}}})
		fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", b)
	}))
	defer server.Close()
	t.Run("code", func(t *testing.T) {
		inputs = nil
		code, out, errs := runForTest(t, []string{"--server", server.URL, "--translate-code"}, "```text\nhello\n```\n")
		if code != 0 || out != "```text\n訳\"\nhello\n```\n" || !reflect.DeepEqual(inputs, []string{"hello"}) {
			t.Fatal(code, out, errs, inputs)
		}
	})
	t.Run("json", func(t *testing.T) {
		inputs = nil
		src := "{ \"key\":\"hello\", \"key\":[\"world\",\"\",12345678901234567890,1e+03,true,null,{\"nested\":\"bye\"}] }\n"
		path := filepath.Join(t.TempDir(), "input.json")
		output := filepath.Join(t.TempDir(), "output.json")
		if err := os.WriteFile(path, []byte(src), 0600); err != nil {
			t.Fatal(err)
		}
		code, _, errs := runForTest(t, []string{"--server", server.URL, "--format", "json", "-f", path, "-o", output}, "")
		b, err := os.ReadFile(output)
		expected := src
		for _, value := range []string{"hello", "world", "bye"} {
			encoded, _ := json.Marshal("訳\"\n" + value)
			expected = strings.Replace(expected, "\""+value+"\"", string(encoded), 1)
		}
		if code != 0 || err != nil || string(b) != expected || !json.Valid(b) || !reflect.DeepEqual(inputs, []string{"hello", "world", "bye"}) {
			t.Fatal(code, string(b), errs, err, inputs)
		}
	})
	t.Run("reject before request", func(t *testing.T) {
		for _, src := range []string{`{"broken":`, `{"ok":"hello","bad":"\u003c|plamo:tag|>"}`} {
			inputs = nil
			code, _, errs := runForTest(t, []string{"--server", server.URL, "--format", "json"}, src)
			if code != 1 || len(inputs) != 0 {
				t.Fatal(code, errs, inputs)
			}
		}
		inputs = nil
		code, _, errs := runForTest(t, []string{"--server", server.URL, "--translate-code"}, "```\n<|plamo:tag|>\n```\n")
		if code != 1 || len(inputs) != 0 {
			t.Fatal(code, errs, inputs)
		}
	})
	t.Run("stdin and repl", func(t *testing.T) {
		for _, extra := range [][]string{nil, {"-i"}} {
			args := append([]string{"--server", server.URL, "--format", "json"}, extra...)
			code, out, errs := runForTest(t, args, "[\"hello\"]\n\n")
			if code != 0 || !json.Valid([]byte(out)) {
				t.Fatal(code, out, errs)
			}
		}
	})
}

func TestFormatArguments(t *testing.T) {
	if code, _, _ := runForTest(t, []string{"--format", "yaml"}, ""); code != 2 {
		t.Fatal(code)
	}
}

func TestCodeTranslationChunking(t *testing.T) {
	source := "```text\n" + strings.Repeat("あ", 4500) + "\n```\n"
	count := 0
	for _, part := range splitDocument(source, 2000, true) {
		if part.Translatable {
			count++
			if len([]rune(part.Text)) > 2000 {
				t.Fatal("oversized code chunk")
			}
		}
	}
	if count != 3 {
		t.Fatal("code was not chunked", count)
	}
}

func TestJSONValueSpans(t *testing.T) {
	for _, src := range []string{`"hello"`, `["a\\\"b", {"key:with:colons":"value"}]`, `null`, `42`, `{}`} {
		values, err := jsonValues(src)
		if err != nil {
			t.Fatal(src, err)
		}
		for _, value := range values {
			var decoded string
			if err := json.Unmarshal([]byte(src[value.start:value.end]), &decoded); err != nil || decoded != value.text {
				t.Fatal(src, value, err)
			}
		}
	}
}
