package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/chzyer/readline"
)

const maxInputChars = 2000000

func readLimited(r io.Reader) (string, error) {
	// A UTF-8 character occupies at most four bytes; cap allocation even for hostile inputs.
	b, err := io.ReadAll(io.LimitReader(r, 4*maxInputChars+1))
	if err != nil {
		return "", err
	}
	if !utf8.Valid(b) {
		return "", fmt.Errorf("input is not valid UTF-8")
	}
	s := strings.ReplaceAll(strings.ReplaceAll(string(b), "\r\n", "\n"), "\r", "\n")
	if len(b) > 4*maxInputChars || utf8.RuneCountInString(s) > maxInputChars {
		return "", fmt.Errorf("input is larger than %d chars", maxInputChars)
	}
	return s, nil
}
func readFileLimited(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	return readLimited(f)
}
func readArg(v string) (string, error) {
	if st, err := os.Stat(v); err == nil && st.Mode().IsRegular() {
		return readFileLimited(v)
	}
	return v, nil
}
func parseGlossary(items []string) ([]glossaryEntry, error) {
	var out []glossaryEntry
	for _, it := range items {
		lines := []string{it}
		if !strings.Contains(it, "=>") {
			st, err := os.Stat(it)
			if err != nil || !st.Mode().IsRegular() {
				return nil, fmt.Errorf("glossary must be 'src=>dst' or a file: %s", it)
			}
			s, err := readFileLimited(it)
			if err != nil {
				return nil, err
			}
			lines = strings.Split(s, "\n")
		}
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			s, t, ok := strings.Cut(line, "=>")
			s = strings.TrimSpace(s)
			t = strings.TrimSpace(t)
			if !ok || s == "" || t == "" {
				return nil, fmt.Errorf("bad glossary line: %s", line)
			}
			out = append(out, glossaryEntry{s, t})
		}
	}
	return out, nil
}

type safeWriter struct{ out io.Writer }

func (w safeWriter) Write(b []byte) (int, error) {
	s := strings.Map(func(r rune) rune {
		if r <= 8 || (r >= 11 && r <= 31) || (r >= 127 && r <= 159) {
			return -1
		}
		return r
	}, string(b))
	_, err := io.WriteString(w.out, s)
	if err != nil {
		return 0, err
	}
	return len(b), nil
}
func isTerminal(f *os.File) bool { return readline.IsTerminal(int(f.Fd())) }

func atomicWrite(path string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, err = f.Write(b)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), path)
}
