package main

import (
	"io"
	"reflect"
	"strings"
	"testing"
)

func TestParseArguments(t *testing.T) {
	a, err := parseArgs([]string{"hello", "-t", "en", "world", "--glossary", "a=>b", "--glossary=c=>d", "-iv", "-sja"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a.text, []string{"hello", "world"}) || a.opt.Target != "en" || a.opt.Source != "ja" || !a.interactive || !a.verbose || len(a.glossary) != 2 {
		t.Fatalf("%+v", a)
	}
	a, err = parseArgs([]string{"--", "-text"}, io.Discard)
	if err != nil || len(a.text) != 1 || a.text[0] != "-text" {
		t.Fatal(a, err)
	}
	for _, args := range [][]string{{"--bad"}, {"--ctx", "abc"}, {"--quant", "bad"}, {"--ctx", "0"}, {"--target"}} {
		if _, err := parseArgs(args, io.Discard); err == nil {
			t.Fatal("accepted", args)
		}
	}
}
func TestSizeArguments(t *testing.T) {
	a, err := parseArgs(nil, io.Discard)
	if err != nil || a.size != "0.8b" || a.quant != "Q4_K_M" || a.definition.Repo != "pfnet/plamo-3-translate-0.8b-GGUF" {
		t.Fatal(a, err)
	}
	for _, size := range []string{"0.8b", "2b", "8b", "31b", "8B"} {
		a, err := parseArgs([]string{"text", "--size", size}, io.Discard)
		if err != nil || a.size != strings.ToLower(size) || a.definition.Size != a.size {
			t.Fatal(a, err)
		}
	}
	for _, args := range [][]string{{"--size", "unknown"}, {"--size", "8b", "--quant", "Q4_0"}, {"--size", "31b", "--quant", "Q4_0"}, {"--size"}} {
		if _, err := parseArgs(args, io.Discard); err == nil {
			t.Fatal("accepted invalid selection", args)
		}
	}
	// Built-in quantization availability does not constrain explicitly supplied models/servers.
	for _, override := range []string{"--model", "--server"} {
		if _, err := parseArgs([]string{"--size", "8b", "--quant", "Q4_0", override, "custom"}, io.Discard); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HF_HUB_CACHE", t.TempDir())
	code, _, errs := runForTest(t, []string{"--size", "8b", "--offline", "hello"}, "")
	if code != 1 || !strings.Contains(errs, "plamo-3-translate-8b-Q4_K_M.gguf") || strings.Contains(errs, "starting llama server") {
		t.Fatal(code, errs)
	}
}
