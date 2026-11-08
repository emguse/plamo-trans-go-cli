package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
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
