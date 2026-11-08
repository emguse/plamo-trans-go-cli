package main

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestDocumentPreservesFormatting(t *testing.T) {
	for _, doc := range []string{"", "abc", "abc\n", "abc\n\n", "\n\nabc\n", "# Title\n\nPara one\nline two\n\n\n```py\nx = 1\n\ny = 2\n```\n\nLast para", "```go\nx\n```", "```go\nx\n```\n", "before\n```\ncode\n```\nafter", "unterminated\n```py\nx\n", "a\n \t\nb"} {
		var got strings.Builder
		for _, s := range splitDocument(doc, 2000) {
			got.WriteString(s.Text + s.Separator)
			if strings.HasPrefix(s.Text, "```") && s.Translatable {
				t.Fatal("code translated", s)
			}
		}
		if got.String() != doc {
			t.Errorf("roundtrip: %q => %q", doc, got.String())
		}
	}
}
func TestLongDocumentBounds(t *testing.T) {
	for _, text := range []string{strings.Repeat("word ", 6201), strings.Repeat("あ", 31000), strings.Repeat("一文です。 ", 1000)} {
		var raw strings.Builder
		for _, s := range splitDocument(text, 2000) {
			if utf8.RuneCountInString(s.Text) > 2000 {
				t.Fatal("unbounded segment")
			}
			raw.WriteString(s.Text)
		}
		if raw.String() != text {
			t.Fatal("source characters lost")
		}
	}
	text := strings.Repeat("word ", 400) + "x"
	if strings.Join(hardSplit(text, 2000), "") != text {
		t.Fatal("hard split lost source")
	}
}
func TestTrimHistory(t *testing.T) {
	h := make([]pair, 10)
	for i := range h {
		h[i] = pair{"a", "b"}
	}
	if len(trimHistory(h)) != 4 {
		t.Fatal("pairs")
	}
	h = append(h, pair{strings.Repeat("あ", 3000), "x"})
	if len(trimHistory(h)) != 0 {
		t.Fatal("character budget")
	}
}

func TestInlineReservedTokens(t *testing.T) {
	for _, doc := range []string{
		"Reserved token `<|plamo:` is rejected.\n",
		"``<|plamo:tag|> `example` `` and `<|plamo:bos|>` are examples.",
		"`<|plamo:`", " `<|plamo:` ", "a\n`<|plamo:`\nb",
		strings.Repeat("あ", 1999) + " `<|plamo:tag|>` tail",
	} {
		var got strings.Builder
		protected := false
		for _, s := range splitDocument(doc, 2000) {
			got.WriteString(s.Text + s.Separator)
			if s.Translatable {
				if err := checkText(s.Text, "input", maxRequestChars); err != nil {
					t.Fatal(err)
				}
			} else if strings.Contains(s.Text, "<|plamo:") {
				protected = true
			}
		}
		if got.String() != doc {
			t.Fatalf("formatting changed: %q => %q", doc, got.String())
		}
		if !protected {
			t.Fatalf("not protected: %q", doc)
		}
	}
	// Escaped, unmatched, and mismatched delimiters do not exempt ordinary text.
	for _, doc := range []string{"token <|plamo:tag|>", "`<|plamo:", "\\`<|plamo:tag|>`", "``<|plamo:tag|>`"} {
		rejected := false
		for _, s := range splitDocument(doc, 2000) {
			if s.Translatable && checkText(s.Text, "input", maxRequestChars) != nil {
				rejected = true
			}
		}
		if !rejected {
			t.Fatalf("ordinary token was exempted: %q", doc)
		}
	}
}
