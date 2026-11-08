package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestReadLimitsAndSafeOutput(t *testing.T) {
	for _, s := range []string{strings.Repeat("x", maxInputChars+1), strings.Repeat("あ", maxInputChars+1), "\xff"} {
		if _, err := readLimited(strings.NewReader(s)); err == nil {
			t.Fatal("accepted excessive or invalid input")
		}
	}
	s, err := readLimited(strings.NewReader("あ\r\nb\rc"))
	if err != nil || s != "あ\nb\nc" {
		t.Fatal(s, err)
	}
	var out bytes.Buffer
	w := safeWriter{&out}
	text := "a\x1b[31mb\tc\nd\u009be\x07"
	n, err := w.Write([]byte(text))
	if err != nil || n != len(text) || out.String() != "a[31mb\tc\nde" {
		t.Fatal(n, err, out.String())
	}
}

func TestGlossary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "glossary.txt")
	os.WriteFile(path, []byte("# comment\n管理画面 => dashboard\n\nx=>y\n"), 0600)
	got, err := parseGlossary([]string{path, "a=>b"})
	if err != nil || !reflect.DeepEqual(got, []glossaryEntry{{"管理画面", "dashboard"}, {"x", "y"}, {"a", "b"}}) {
		t.Fatal(got, err)
	}
	for _, s := range []string{"bad", "=>b", "a=>"} {
		if _, err := parseGlossary([]string{s}); err == nil {
			t.Fatal("accepted", s)
		}
	}
}
