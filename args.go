package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

type stringList []string

func (s *stringList) String() string     { return strings.Join(*s, ",") }
func (s *stringList) Set(v string) error { *s = append(*s, v); return nil }

type arguments struct {
	definition                                      modelDefinition
	opt                                             options
	glossary                                        stringList
	file, output, size, quant, model, server, llama string
	interactive, offline, verbose                   bool
	ctx                                             int
	text                                            []string
}

func parseArgs(args []string, stderr io.Writer) (arguments, error) {
	var a arguments
	fs := flag.NewFlagSet("plamo-trans", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintln(stderr, "Usage: plamo-trans [options] [text ...]\nTranslate with PLaMo 3 Translate (GGUF).\nText defaults to stdin. Options may appear before or after text.")
		fs.PrintDefaults()
	}
	for _, n := range []string{"t", "target"} {
		fs.StringVar(&a.opt.Target, n, "", "target language (default: ja <-> en auto)")
	}
	for _, n := range []string{"s", "source"} {
		fs.StringVar(&a.opt.Source, n, "", "source language (requires --target)")
	}
	fs.StringVar(&a.opt.Form, "form", "", "Japanese style: polite or plain (requires -t ja)")
	fs.Var(&a.glossary, "glossary", "repeatable FILE or 'source=>target'")
	fs.StringVar(&a.opt.Context, "context", "", "reference text or file path")
	for _, n := range []string{"f", "file"} {
		fs.StringVar(&a.file, n, "", "text/Markdown file to translate")
	}
	for _, n := range []string{"o", "output"} {
		fs.StringVar(&a.output, n, "", "output file for --file")
	}
	for _, n := range []string{"i", "interactive"} {
		fs.BoolVar(&a.interactive, n, false, "REPL mode (blank line to translate)")
	}
	fs.StringVar(&a.size, "size", defaultModelSize, "model size: "+strings.Join(modelSizes(), ", "))
	fs.StringVar(&a.quant, "quant", defaultQuant, "quantization: Q4_K_M, Q4_0 (0.8b/2b only), Q3_K_M, Q2_K")
	fs.StringVar(&a.model, "model", "", "local GGUF path")
	fs.BoolVar(&a.offline, "offline", false, "disable model downloads and start llama offline")
	fs.StringVar(&a.server, "server", "", "use an existing server URL")
	fs.StringVar(&a.llama, "llama-bin", "", "path to llama or llama-server")
	fs.IntVar(&a.ctx, "ctx", 16384, "llama context size")
	fs.IntVar(&a.opt.MaxTokens, "max-tokens", 2048, "maximum output tokens")
	for _, n := range []string{"v", "verbose"} {
		fs.BoolVar(&a.verbose, n, false, "show llama server logs")
	}
	// flag stops at the first positional argument; collect flags separately for argparse compatibility.
	var flags, text []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			text = append(text, args[i+1:]...)
			break
		}
		if arg == "-" || !strings.HasPrefix(arg, "-") {
			text = append(text, arg)
			continue
		}
		name := strings.TrimLeft(arg, "-")
		name, _, hasValue := strings.Cut(name, "=")
		f := fs.Lookup(name)
		// argparse also accepts attached short values (-ten, -fdoc.md) and boolean clusters (-iv).
		if f == nil && !strings.HasPrefix(arg, "--") && len(arg) > 2 && !hasValue {
			first := fs.Lookup(arg[1:2])
			if first != nil {
				if _, ok := first.Value.(interface{ IsBoolFlag() bool }); !ok {
					flags = append(flags, arg[:2], arg[2:])
					continue
				}
				valid := true
				for _, ch := range arg[1:] {
					v := fs.Lookup(string(ch))
					if v == nil {
						valid = false
						break
					}
					if _, ok := v.Value.(interface{ IsBoolFlag() bool }); !ok {
						valid = false
						break
					}
				}
				if valid {
					for _, ch := range arg[1:] {
						flags = append(flags, "-"+string(ch))
					}
					continue
				}
			}
		}
		flags = append(flags, arg)
		if f != nil && !hasValue {
			if _, ok := f.Value.(interface{ IsBoolFlag() bool }); !ok {
				if i+1 >= len(args) {
					return a, fmt.Errorf("flag needs an argument: %s", arg)
				}
				i++
				flags = append(flags, args[i])
			}
		}
	}
	if err := fs.Parse(flags); err != nil {
		return a, err
	}
	a.text = text
	definition, err := lookupModel(a.size)
	if err != nil {
		return a, err
	}
	a.size = definition.Size
	a.definition = definition
	if !knownQuant(a.quant) {
		return a, fmt.Errorf("invalid --quant %q", a.quant)
	}
	// Local paths and external servers bypass built-in model selection.
	if a.model == "" && a.server == "" {
		if _, err := definition.filename(a.quant); err != nil {
			return a, err
		}
	}
	if a.ctx <= 0 {
		return a, fmt.Errorf("--ctx must be positive")
	}
	a.offline = a.offline || os.Getenv("HF_HUB_OFFLINE") == "1"
	return a, nil
}
