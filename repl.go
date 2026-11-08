package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/chzyer/readline"
)

// readBlock returns a complete input at a blank line, a leading command, or EOF.
func readBlock(next func(string) (string, error)) (string, error) {
	var lines []string
	n := 0
	for {
		prompt := "> "
		if len(lines) > 0 {
			prompt = "  "
		}
		line, err := next(prompt)
		if err != nil && err != io.EOF {
			return "", err
		}
		if !utf8.ValidString(line) {
			return "", fmt.Errorf("input is not valid UTF-8")
		}
		if line != "" {
			if len(lines) == 0 && strings.HasPrefix(strings.TrimSpace(line), "/") {
				return strings.TrimSpace(line), nil
			}
			if strings.TrimSpace(line) != "" {
				n += utf8.RuneCountInString(line)
				if len(lines) > 0 {
					n++
				}
				if n > maxRequestChars {
					return "", fmt.Errorf("input is too long; use -f to translate in chunks")
				}
				lines = append(lines, line)
			} else if len(lines) > 0 {
				return strings.Join(lines, "\n"), nil
			}
		} else if len(lines) > 0 {
			return strings.Join(lines, "\n"), nil
		}
		if err == io.EOF {
			if len(lines) > 0 {
				return strings.Join(lines, "\n"), nil
			}
			return "", io.EOF
		}
	}
}
func repl(ctx context.Context, c *client, o options, in *os.File, out, stderr io.Writer) error {
	var editor *readline.Instance
	reader := bufio.NewScanner(in)
	reader.Buffer(make([]byte, 4096), 4*maxRequestChars+1)
	next := func(prompt string) (string, error) {
		if reader.Scan() {
			return reader.Text(), nil
		}
		if err := reader.Err(); err != nil {
			return "", err
		}
		return "", io.EOF
	}
	if isTerminal(in) {
		base := os.Getenv("XDG_STATE_HOME")
		if base == "" {
			h, _ := os.UserHomeDir()
			base = filepath.Join(h, ".local", "state")
		}
		hist := filepath.Join(base, "plamo-trans", "history")
		if err := os.MkdirAll(filepath.Dir(hist), 0700); err != nil {
			hist = ""
		}
		// Only editor-generated cursor sequences bypass the terminal output filter.
		editorOut := stderr
		if w, ok := stderr.(safeWriter); ok {
			editorOut = w.out
		}
		if hist != "" {
			f, e := os.OpenFile(hist, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
			if e != nil {
				hist = ""
			} else {
				f.Close()
			}
		}
		input := readline.NewCancelableStdin(in)
		var err error
		editor, err = readline.NewEx(&readline.Config{Prompt: "> ", HistoryFile: hist, HistoryLimit: 1000, Stdin: input, Stdout: editorOut, Stderr: editorOut, FuncGetWidth: func() int {
			w := readline.GetScreenWidth()
			if w <= 0 {
				return 80
			}
			return w
		}, InterruptPrompt: "^C", EOFPrompt: "\n"})
		if err != nil {
			input.Close()
			return err
		}
		var once sync.Once
		closeEditor := func() { once.Do(func() { input.Close(); editor.Close() }) }
		stopClose := context.AfterFunc(ctx, closeEditor)
		defer func() { stopClose(); closeEditor(); compactHistory(hist) }()
		next = func(prompt string) (string, error) { editor.SetPrompt(prompt); return editor.Readline() }
	}
	fmt.Fprintln(stderr, "plamo-trans REPL  (blank line to translate; /target LANG, /clear, /quit)")
	var history []pair
	eof := false
	readLine := func(prompt string) (string, error) {
		if eof {
			return "", io.EOF
		}
		line, err := next(prompt)
		if err == io.EOF {
			eof = true
		}
		return line, err
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		line, err := readBlock(readLine)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err == io.EOF || errors.Is(err, readline.ErrInterrupt) {
			return nil
		}
		if err != nil {
			return err
		}
		if line == "/quit" || line == "/exit" {
			return nil
		}
		if line == "/clear" {
			history = nil
			continue
		}
		fields := strings.Fields(line)
		if len(fields) > 0 && fields[0] == "/target" {
			candidate := o
			candidate.Target = ""
			if len(fields) > 1 {
				candidate.Target = strings.Join(fields[1:], " ")
			}
			if err := candidate.validate(); err != nil {
				fmt.Fprintln(stderr, "error:", err)
			} else {
				o = candidate
				history = nil
			}
			continue
		}
		tr, err := c.stream(ctx, line, trimHistory(history), o, out)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			fmt.Fprintln(stderr, "error:", err)
			continue
		}
		if _, err := fmt.Fprintln(out); err != nil {
			return err
		}
		history = append(trimHistory(history), pair{line, tr})
	}
}

// readline bounds its in-memory history; also bound the persisted history on exit.
func compactHistory(path string) {
	if path == "" {
		return
	}
	f, err := os.Open(path)
	if err != nil {
		return
	}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 4096), 4*maxRequestChars+1)
	lines := make([]string, 1000)
	count := 0
	for scanner.Scan() {
		if scanner.Text() != "" {
			lines[count%1000] = scanner.Text()
			count++
		}
	}
	err = scanner.Err()
	f.Close()
	if err != nil {
		return
	}
	start := 0
	if count > 1000 {
		start = count % 1000
		count = 1000
	}
	var b strings.Builder
	for i := 0; i < count; i++ {
		b.WriteString(lines[(start+i)%1000])
		b.WriteByte('\n')
	}
	_ = atomicWrite(path, []byte(b.String()))
}
