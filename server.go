package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

func findLlama(explicit string) ([]string, error) {
	candidates := []string{explicit, os.Getenv("PLAMO_LLAMA_BIN")}
	for _, c := range candidates {
		if c != "" {
			p, err := exec.LookPath(c)
			if err != nil {
				return nil, fmt.Errorf("llama binary not found: %s", c)
			}
			return llamaPrefix(p), nil
		}
	}
	for _, c := range []string{"llama", "llama-server"} {
		if p, err := exec.LookPath(c); err == nil {
			return llamaPrefix(p), nil
		}
	}
	return nil, fmt.Errorf("`llama` / `llama-server` not found in PATH; use --llama-bin or PLAMO_LLAMA_BIN")
}
func llamaPrefix(path string) []string {
	if strings.HasPrefix(filepath.Base(path), "llama-server") {
		return []string{path}
	}
	return []string{path, "serve"}
}

type server struct {
	base string
	cmd  *exec.Cmd
	done chan struct{}
	once sync.Once
}

func startServer(ctx context.Context, prefix []string, model string, ctxSize int, verbose, offline bool, stderr io.Writer) (*server, error) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	args := append(append([]string{}, prefix[1:]...), "-m", model, "-c", strconv.Itoa(ctxSize), "--host", "127.0.0.1", "--port", strconv.Itoa(port), "--alias", modelAlias, "--jinja", "--no-prefill-assistant")
	if offline {
		args = append(args, "--offline")
	}
	cmd := exec.Command(prefix[0], args...)
	configureProcess(cmd)
	if verbose {
		cmd.Stdout = stderr
		cmd.Stderr = stderr
	}
	if offline {
		cmd.Env = append(os.Environ(), "HF_HUB_OFFLINE=1", "TRANSFORMERS_OFFLINE=1")
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	s := &server{base: fmt.Sprintf("http://127.0.0.1:%d", port), cmd: cmd, done: make(chan struct{})}
	go func() { _ = cmd.Wait(); close(s.done) }()
	healthClient := newInferenceHTTPClient(time.Second)
	defer healthClient.CloseIdleConnections()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	timer := time.NewTimer(120 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			s.stop()
			return nil, ctx.Err()
		case <-s.done:
			s.stop()
			return nil, fmt.Errorf("llama server exited early (%s); rerun with --verbose", cmd.ProcessState)
		case <-timer.C:
			s.stop()
			return nil, fmt.Errorf("timed out waiting for llama server")
		case <-ticker.C:
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, s.base+"/health", nil)
			res, err := healthClient.Do(req)
			if err == nil {
				res.Body.Close()
				if res.StatusCode == 200 {
					return s, nil
				}
			}
		}
	}
}
func (s *server) stop() {
	s.once.Do(func() {
		terminateProcess(s.cmd, false)
		select {
		case <-s.done:
		case <-time.After(5 * time.Second):
			terminateProcess(s.cmd, true)
			<-s.done
		}
		// Also clean up descendants if the parent already exited.
		terminateProcess(s.cmd, true)
	})
}
