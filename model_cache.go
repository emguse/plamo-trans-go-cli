package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var commitPattern = regexp.MustCompile(`^[a-fA-F0-9]{40,64}$`)

func hfHome() string {
	if v := os.Getenv("HF_HOME"); v != "" {
		return v
	}
	h, _ := os.UserHomeDir()
	return filepath.Join(h, ".cache", "huggingface")
}
func hubCache() string {
	if v := os.Getenv("HF_HUB_CACHE"); v != "" {
		return v
	}
	return filepath.Join(hfHome(), "hub")
}
func hfToken() string {
	if v := os.Getenv("HF_TOKEN"); v != "" {
		return v
	}
	if v := os.Getenv("HUGGING_FACE_HUB_TOKEN"); v != "" {
		return v
	}
	p := os.Getenv("HF_TOKEN_PATH")
	if p == "" {
		p = filepath.Join(hfHome(), "token")
	}
	b, _ := os.ReadFile(p)
	return strings.TrimSpace(string(b))
}
func resolveModel(ctx context.Context, model modelDefinition, quant string, offline bool, stderr io.Writer) (string, error) {
	name, err := model.filename(quant)
	if err != nil {
		return "", err
	}
	repoDir := filepath.Join(hubCache(), "models--"+strings.ReplaceAll(model.Repo, "/", "--"))
	ref := filepath.Join(repoDir, "refs", "main")
	if b, err := os.ReadFile(ref); err == nil {
		commit := strings.TrimSpace(string(b))
		if commitPattern.MatchString(commit) {
			p := filepath.Join(repoDir, "snapshots", commit, name)
			if st, err := os.Stat(p); err == nil && st.Mode().IsRegular() {
				return p, nil
			}
		}
	}
	if offline {
		return "", fmt.Errorf("%s not found in HF cache (%s) and --offline is set", name, repoDir)
	}
	fmt.Fprintf(stderr, "downloading %s ...\n", name)
	endpoint := strings.TrimRight(os.Getenv("HF_ENDPOINT"), "/")
	if endpoint == "" {
		endpoint = "https://huggingface.co"
	}
	token := hfToken()
	request := func(method, url string) (*http.Request, error) {
		r, e := http.NewRequestWithContext(ctx, method, url, nil)
		if e == nil && token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		return r, e
	}
	// Inspect the resolve response before redirects so the snapshot is pinned to its commit.
	headClient := &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	req, err := request(http.MethodHead, endpoint+"/"+model.Repo+"/resolve/main/"+name)
	if err != nil {
		return "", err
	}
	res, err := headClient.Do(req)
	if err != nil {
		return "", err
	}
	res.Body.Close()
	if res.StatusCode != 200 && res.StatusCode != 302 && res.StatusCode != 307 && res.StatusCode != 301 {
		return "", fmt.Errorf("model metadata: HTTP %d", res.StatusCode)
	}
	commit := res.Header.Get("X-Repo-Commit")
	if !commitPattern.MatchString(commit) {
		return "", fmt.Errorf("model metadata has no valid X-Repo-Commit")
	}
	dest := filepath.Join(repoDir, "snapshots", commit, name)
	if st, err := os.Stat(dest); err == nil && st.Mode().IsRegular() {
		if err := atomicWrite(ref, []byte(commit)); err != nil {
			return "", err
		}
		return dest, nil
	}
	req, err = request(http.MethodGet, endpoint+"/"+model.Repo+"/resolve/"+commit+"/"+name)
	if err != nil {
		return "", err
	}
	downloadClient := &http.Client{} // Model transfers may take longer than inference requests; cancellation uses ctx.
	res, err = downloadClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return "", fmt.Errorf("model download: HTTP %d", res.StatusCode)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(filepath.Dir(dest), ".download-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(f.Name())
	n, copyErr := io.Copy(f, res.Body)
	closeErr := f.Close()
	if copyErr != nil {
		return "", copyErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	if n == 0 {
		return "", fmt.Errorf("model download is empty")
	}
	if err := os.Rename(f.Name(), dest); err != nil {
		return "", err
	}
	if err := atomicWrite(ref, []byte(commit)); err != nil {
		return "", err
	}
	return dest, nil
}
