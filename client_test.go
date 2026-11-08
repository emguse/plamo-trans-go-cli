package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"reflect"
	"strings"
	"testing"
)

func TestOptionsAndSafety(t *testing.T) {
	for _, o := range []options{{Source: "ja", MaxTokens: 1}, {Target: "en", Form: "polite", MaxTokens: 1}, {Target: "ja", Form: "bad", MaxTokens: 1}, {Context: "<|plamo:bos|>", MaxTokens: 1}, {Glossary: []glossaryEntry{{"a", strings.Repeat("あ", 501)}}, MaxTokens: 1}, {MaxTokens: 0}} {
		if err := o.validate(); err == nil {
			t.Errorf("accepted invalid options: %+v", o)
		}
	}
	o := options{Target: " JA ", Source: "en", Form: "polite", MaxTokens: 2048, Glossary: []glossaryEntry{{"a", "b"}}}
	if err := o.validate(); err != nil {
		t.Fatal(err)
	}
	if o.Target != "ja" || normLang("JA(EASY)") != "ja-easy" {
		t.Fatal(o)
	}
	if !reflect.DeepEqual(o.kwargs(), map[string]any{"target_lang": "ja", "source_lang": "en", "form": "polite", "glossary": o.Glossary}) {
		t.Fatal(o.kwargs())
	}
	if err := checkText(strings.Repeat("あ", 30000), "input", 30000); err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{strings.Repeat("あ", 30001), "x<|plamo:tag|>"} {
		if checkText(s, "input", 30000) == nil {
			t.Fatal("accepted unsafe input")
		}
	}
}
func TestClientPayloadAndStreaming(t *testing.T) {
	var bodies []map[string]any
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/prefix/v1/chat/completions" || r.Method != "POST" || r.Header.Get("Content-Type") != "application/json" {
			t.Error(r.URL, r.Method)
		}
		var b map[string]any
		if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
			t.Error(err)
		}
		bodies = append(bodies, b)
		if b["stream"] == true {
			fmt.Fprint(w, ": keepalive\n\ndata: {\"choices\":[{\"delta\":{\"role\":\"assistant\"}}]}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\"こん\"}}]}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\"にちは\"}}]}\n\ndata: [DONE]\n\n")
		} else {
			fmt.Fprint(w, `{"choices":[{"message":{"content":"hello"}}]}`)
		}
	}))
	defer s.Close()
	c := newClient(s.URL + "/prefix/")
	o := options{Target: "en", MaxTokens: 2048}
	tr, err := c.translate(context.Background(), "本文", o)
	if err != nil || tr != "hello" {
		t.Fatal(tr, err)
	}
	var out bytes.Buffer
	tr, err = c.stream(context.Background(), "text", []pair{{"previous", "前の文"}}, o, &out)
	if err != nil || tr != "こんにちは" || out.String() != tr {
		t.Fatal(tr, err, out.String())
	}
	if bodies[0]["temperature"] != float64(0) || bodies[0]["model"] != modelAlias || bodies[0]["max_tokens"] != float64(2048) {
		t.Fatal(bodies)
	}
	msgs := bodies[1]["messages"].([]any)
	if len(msgs) != 3 || msgs[0].(map[string]any)["content"] != "previous" || msgs[1].(map[string]any)["role"] != "assistant" {
		t.Fatal(msgs)
	}
	if bodies[0]["chat_template_kwargs"].(map[string]any)["target_lang"] != "en" {
		t.Fatal(bodies)
	}
}
func TestClientErrors(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
		stream bool
	}{{500, "failure", false}, {200, `{"choices":[]}`, false}, {200, `{`, false}, {200, "data: {oops}\n", true}, {200, "data: {\"choices\":[]}\n", true}} {
		t.Run(fmt.Sprint(tc), func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status); fmt.Fprint(w, tc.body) }))
			defer s.Close()
			c := newClient(s.URL)
			o := options{MaxTokens: 20}
			var err error
			if tc.stream {
				_, err = c.stream(context.Background(), "a", nil, o, io.Discard)
			} else {
				_, err = c.translate(context.Background(), "a", o)
			}
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := newClient("http://127.0.0.1:1").translate(ctx, "a", options{}); err == nil {
		t.Fatal("ignored cancellation")
	}
}

// Close the server-side connection exactly when a client tries to reuse it.
// Every translation must instead get a fresh connection, also when the health
// endpoint has left a connection in the default transport's pool.
func TestClientDoesNotReuseClosingIdleConnections(t *testing.T) {
	var s *httptest.Server
	s = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			fmt.Fprint(w, "ok")
			return
		}
		fmt.Fprint(w, `{"choices":[{"message":{"content":"translated"}}]}`)
	}))
	defer s.Close()
	health, err := http.Get(s.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, health.Body)
	health.Body.Close()
	defer http.DefaultClient.CloseIdleConnections()
	c := newClient(s.URL)
	defer c.http.CloseIdleConnections()
	reused := false
	ctx := httptrace.WithClientTrace(context.Background(), &httptrace.ClientTrace{GotConn: func(info httptrace.GotConnInfo) {
		if info.Reused {
			reused = true
			s.CloseClientConnections()
		}
	}})
	for i := 0; i < 5; i++ {
		tr, err := c.translate(ctx, "text", options{MaxTokens: 20})
		if err != nil || tr != "translated" {
			t.Fatalf("request %d: %q, %v", i, tr, err)
		}
	}
	if reused {
		t.Fatal("reused an idle connection")
	}
}

func TestInterruptedStreamIsNotReplayed(t *testing.T) {
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n")
		// End the response without [DONE], as happens on a broken stream.
	}))
	defer s.Close()
	c := newClient(s.URL)
	defer c.http.CloseIdleConnections()
	var out bytes.Buffer
	tr, err := c.stream(context.Background(), "text", nil, options{MaxTokens: 20}, &out)
	if !errors.Is(err, io.ErrUnexpectedEOF) || tr != "partial" || out.String() != "partial" || calls != 1 {
		t.Fatal(tr, err, out.String(), calls)
	}
}
