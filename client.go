package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

const maxRequestChars = 30000
const modelAlias = "plamo-3-translate"

type glossaryEntry struct {
	Source string `json:"source"`
	Target string `json:"target"`
}
type pair struct{ Source, Translation string }
type options struct {
	Target, Source, Form, Context string
	Glossary                      []glossaryEntry
	MaxTokens                     int
}

func normLang(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "ja(easy)" {
		return "ja-easy"
	}
	return s
}
func (o *options) validate() error {
	o.Target, o.Source = normLang(o.Target), normLang(o.Source)
	if o.Source != "" && (o.Target == "" || o.Target == "auto") {
		return fmt.Errorf("--source requires a concrete --target (e.g. -t en)")
	}
	if o.Form != "" && o.Target != "ja" {
		return fmt.Errorf("--form requires --target ja")
	}
	if o.Form != "" && o.Form != "polite" && o.Form != "plain" {
		return fmt.Errorf("--form must be polite or plain")
	}
	if o.MaxTokens <= 0 {
		return fmt.Errorf("--max-tokens must be positive")
	}
	if err := checkText(o.Context, "context", 8000); err != nil {
		return err
	}
	for _, g := range o.Glossary {
		for _, s := range []string{g.Source, g.Target} {
			if err := checkText(s, "glossary", 500); err != nil {
				return err
			}
		}
	}
	return nil
}
func checkText(s, what string, limit int) error {
	if strings.Contains(s, "<|plamo:") {
		return fmt.Errorf("%s contains reserved token '<|plamo:'", what)
	}
	if n := utf8.RuneCountInString(s); n > limit {
		return fmt.Errorf("%s is too long (%d > %d chars); use -f to translate in chunks", what, n, limit)
	}
	return nil
}
func (o options) kwargs() map[string]any {
	kw := map[string]any{}
	if o.Target != "" && o.Target != "auto" {
		kw["target_lang"] = o.Target
	}
	if o.Source != "" {
		kw["source_lang"] = o.Source
	}
	if o.Form != "" {
		kw["form"] = o.Form
	}
	if o.Context != "" {
		kw["context"] = o.Context
	}
	if len(o.Glossary) > 0 {
		kw["glossary"] = o.Glossary
	}
	return kw
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
type completion struct {
	Choices []struct {
		Message struct {
			Content *string `json:"content"`
		} `json:"message"`
		Delta struct {
			Content *string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
}
type client struct {
	base string
	http *http.Client
}

func newClient(base string) *client {
	return &client{strings.TrimRight(base, "/"), newInferenceHTTPClient(300 * time.Second)}
}

// llama.cpp may close an idle HTTP connection as the next POST starts.
// Use a private transport and a fresh connection per request, including health
// checks. Replaying a POST or a partially emitted stream could duplicate work.
func newInferenceHTTPClient(timeout time.Duration) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DisableKeepAlives = true
	return &http.Client{Transport: transport, Timeout: timeout}
}

func (c *client) request(ctx context.Context, text string, history []pair, o options, stream bool) (*http.Response, error) {
	if err := checkText(text, "input", maxRequestChars); err != nil {
		return nil, err
	}
	msgs := make([]message, 0, len(history)*2+1)
	for _, h := range history {
		msgs = append(msgs, message{"user", h.Source}, message{"assistant", h.Translation})
	}
	msgs = append(msgs, message{"user", text})
	body, err := json.Marshal(map[string]any{"model": modelAlias, "messages": msgs, "temperature": 0, "max_tokens": o.MaxTokens, "stop": []string{"<|plamo:tag|>", "<|plamo:bos|>"}, "stream": stream, "chat_template_kwargs": o.kwargs()})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		defer res.Body.Close()
		b, _ := io.ReadAll(io.LimitReader(res.Body, 1200))
		r := []rune(string(b))
		if len(r) > 300 {
			r = r[:300]
		}
		return nil, fmt.Errorf("server error %d: %s", res.StatusCode, string(r))
	}
	return res, nil
}
func (c *client) translate(ctx context.Context, text string, o options) (string, error) {
	res, err := c.request(ctx, text, nil, o, false)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	var v completion
	if err := json.NewDecoder(res.Body).Decode(&v); err != nil {
		return "", fmt.Errorf("invalid server response: %w", err)
	}
	if len(v.Choices) == 0 || v.Choices[0].Message.Content == nil {
		return "", fmt.Errorf("server response has no translation")
	}
	return *v.Choices[0].Message.Content, nil
}
func (c *client) stream(ctx context.Context, text string, history []pair, o options, out io.Writer) (string, error) {
	res, err := c.request(ctx, text, history, o, true)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	scanner := bufio.NewScanner(res.Body)
	scanner.Buffer(make([]byte, 4096), 8*1024*1024)
	var result strings.Builder
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(line[5:])
		if data == "[DONE]" {
			return result.String(), nil
		}
		if data == "" {
			continue
		}
		var v completion
		if err := json.Unmarshal([]byte(data), &v); err != nil {
			return result.String(), fmt.Errorf("invalid stream response: %w", err)
		}
		if len(v.Choices) == 0 {
			continue
		}
		d := v.Choices[0].Delta.Content
		if d != nil {
			if _, err := io.WriteString(out, *d); err != nil {
				return result.String(), err
			}
			result.WriteString(*d)
		}
	}
	if err := scanner.Err(); err != nil {
		return result.String(), err
	}
	return result.String(), io.ErrUnexpectedEOF
}
