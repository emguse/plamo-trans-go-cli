package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestModelResolution(t *testing.T) {
	for _, model := range modelDefinitions {
		for _, quant := range model.Quants {
			t.Run(model.Size+"/"+quant, func(t *testing.T) {
				cache := t.TempDir()
				t.Setenv("HF_HUB_CACHE", cache)
				t.Setenv("HF_TOKEN", "test-token")
				commit := strings.Repeat("a", 40)
				name := model.FilePrefix + "-" + quant + ".gguf"
				var calls atomic.Int32
				s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					if r.Header.Get("Authorization") != "Bearer test-token" {
						t.Error("missing token")
					}
					if r.Method == http.MethodHead {
						if r.URL.Path != "/"+model.Repo+"/resolve/main/"+name {
							t.Error("wrong metadata path", r.URL.Path)
						}
						w.Header().Set("X-Repo-Commit", commit)
						w.Header().Set("Location", "/unused")
						w.WriteHeader(302)
						return
					}
					if r.URL.Path != "/"+model.Repo+"/resolve/"+commit+"/"+name {
						t.Error("wrong download path", r.URL.Path)
					}
					fmt.Fprint(w, "fake GGUF")
				}))
				defer s.Close()
				t.Setenv("HF_ENDPOINT", s.URL)
				p, err := resolveModel(context.Background(), model, quant, true, io.Discard)
				if err == nil || calls.Load() != 0 {
					t.Fatal("offline fetched", p, err)
				}
				p, err = resolveModel(context.Background(), model, quant, false, io.Discard)
				if err != nil {
					t.Fatal(err)
				}
				repo := filepath.Join(cache, "models--"+strings.ReplaceAll(model.Repo, "/", "--"))
				want := filepath.Join(repo, "snapshots", commit, name)
				b, _ := os.ReadFile(p)
				if p != want || string(b) != "fake GGUF" || calls.Load() != 2 {
					t.Fatal(p, string(b), calls.Load())
				}
				for _, offline := range []bool{false, true} {
					got, err := resolveModel(context.Background(), model, quant, offline, io.Discard)
					if err != nil || got != p || calls.Load() != 2 {
						t.Fatal("cache miss", got, err)
					}
				}
				// Only refs/main is authoritative, even if an older snapshot is present.
				os.WriteFile(filepath.Join(repo, "refs", "main"), []byte(strings.Repeat("b", 40)), 0600)
				if _, err := resolveModel(context.Background(), model, quant, true, io.Discard); err == nil {
					t.Fatal("used stale snapshot")
				}
			})
		}
	}
}
func TestFailedDownloadDoesNotPopulateCache(t *testing.T) {
	model := modelForTest(t, "2b")
	t.Setenv("HF_HUB_CACHE", t.TempDir())
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "HEAD" {
			w.Header().Set("X-Repo-Commit", strings.Repeat("c", 40))
			return
		}
		w.Header().Set("Content-Length", "100")
		fmt.Fprint(w, "short")
	}))
	defer s.Close()
	t.Setenv("HF_ENDPOINT", s.URL)
	if _, err := resolveModel(context.Background(), model, "Q2_K", false, io.Discard); err == nil {
		t.Fatal("accepted truncated download")
	}
	if _, err := resolveModel(context.Background(), model, "Q2_K", true, io.Discard); err == nil {
		t.Fatal("cached partial download")
	}
}
func TestModelCacheIsolation(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("HF_HUB_CACHE", cache)
	small := modelForTest(t, "0.8b")
	commit := strings.Repeat("a", 40)
	repo := filepath.Join(cache, "models--"+strings.ReplaceAll(small.Repo, "/", "--"))
	file := filepath.Join(repo, "snapshots", commit, "plamo-3-translate-0.8b-Q4_K_M.gguf")
	if err := atomicWrite(file, []byte("fake GGUF")); err != nil {
		t.Fatal(err)
	}
	if err := atomicWrite(filepath.Join(repo, "refs", "main"), []byte(commit)); err != nil {
		t.Fatal(err)
	}
	if got, err := resolveModel(context.Background(), small, defaultQuant, true, io.Discard); err != nil || got != file {
		t.Fatal(got, err)
	}
	for _, size := range []string{"2b", "8b", "31b"} {
		m := modelForTest(t, size)
		if _, err := resolveModel(context.Background(), m, defaultQuant, true, io.Discard); err == nil || !strings.Contains(err.Error(), m.FilePrefix) {
			t.Fatal("used another size's cache", size, err)
		}
	}
	for _, size := range []string{"8b", "31b"} {
		if _, err := resolveModel(context.Background(), modelForTest(t, size), "Q4_0", false, io.Discard); err == nil {
			t.Fatal("unsupported quantization reached download")
		}
	}
}
func TestCacheEnvironment(t *testing.T) {
	t.Setenv("HF_HUB_CACHE", "")
	t.Setenv("HF_HOME", t.TempDir())
	if hubCache() != filepath.Join(os.Getenv("HF_HOME"), "hub") {
		t.Fatal(hubCache())
	}
}
