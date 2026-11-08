package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestServerHelper(t *testing.T) {
	if os.Getenv("PLAMO_TEST_SERVER") != "1" {
		return
	}
	if os.Getenv("PLAMO_TEST_EXIT") == "1" {
		os.Exit(3)
	}
	var port string
	for i, a := range os.Args {
		if a == "--port" && i+1 < len(os.Args) {
			port = os.Args[i+1]
		}
	}
	if port == "" {
		os.Exit(4)
	}
	if os.Getenv("HF_HUB_OFFLINE") != "1" || os.Getenv("TRANSFORMERS_OFFLINE") != "1" {
		os.Exit(5)
	}
	if !strings.Contains(strings.Join(os.Args, " "), "--offline") {
		os.Exit(6)
	}
	ln, err := net.Listen("tcp4", "127.0.0.1:"+port)
	if err != nil {
		os.Exit(7)
	}
	http.Serve(ln, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			fmt.Fprint(w, "ok")
			return
		}
		http.NotFound(w, r)
	}))
	os.Exit(0)
}
func TestServerLifecycle(t *testing.T) {
	t.Setenv("PLAMO_TEST_SERVER", "1")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s, err := startServer(ctx, []string{exe, "-test.run=^TestServerHelper$", "--"}, "fake.gguf", 16384, false, true, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	s.stop()
	s.stop()
	select {
	case <-s.done:
	default:
		t.Fatal("server still running")
	}
	t.Setenv("PLAMO_TEST_EXIT", "1")
	if _, err := startServer(ctx, []string{exe, "-test.run=^TestServerHelper$", "--"}, "fake.gguf", 16384, false, true, io.Discard); err == nil {
		t.Fatal("ignored early exit")
	}
}
func TestLlamaPrefix(t *testing.T) {
	for _, name := range []string{"llama", "llama-server", "llama-server-custom"} {
		p := filepath.Join("bin", name)
		want := []string{p}
		if name == "llama" {
			want = append(want, "serve")
		}
		if !reflect.DeepEqual(llamaPrefix(p), want) {
			t.Fatal(name)
		}
	}
	if _, err := findLlama(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing binary accepted")
	}
}
