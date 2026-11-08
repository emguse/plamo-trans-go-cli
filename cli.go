package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

func run(ctx context.Context, args []string, in *os.File, stdout, stderr *os.File) int {
	if !isTerminal(in) {
		stop := context.AfterFunc(ctx, func() { in.Close() })
		defer stop()
	}
	var out io.Writer = stdout
	var errs io.Writer = stderr
	if isTerminal(stdout) {
		out = safeWriter{stdout}
	}
	if isTerminal(stderr) {
		errs = safeWriter{stderr}
	}
	a, err := parseArgs(args, errs)
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	if err != nil {
		fmt.Fprintln(errs, "error:", err)
		return 2
	}
	fail := func(err error) int {
		if ctx.Err() != nil {
			return 130
		}
		fmt.Fprintln(errs, "error:", err)
		return 1
	}
	a.opt.Glossary, err = parseGlossary(a.glossary)
	if err != nil {
		return fail(err)
	}
	if a.opt.Context != "" {
		a.opt.Context, err = readArg(a.opt.Context)
		if err != nil {
			return fail(err)
		}
	}
	if err = a.opt.validate(); err != nil {
		return fail(err)
	}
	var src string
	switch {
	case a.interactive:
	case a.file != "":
		src, err = readFileLimited(a.file)
	case len(a.text) > 0:
		src = strings.Join(a.text, " ")
	case !isTerminal(in):
		src, err = readLimited(in)
	default:
		fmt.Fprintln(errs, "Usage: plamo-trans [options] [text ...] (see --help)")
		return 2
	}
	if err != nil {
		return fail(err)
	}
	if !a.interactive && strings.TrimSpace(src) == "" {
		return 0
	}
	docMode := a.file != "" || len(a.text) == 0 && strings.Contains(strings.TrimSpace(src), "\n")
	if !a.interactive {
		if docMode {
			for _, s := range splitDocument(src, 2000) {
				if s.Translatable {
					if err := checkText(s.Text, "input", maxRequestChars); err != nil {
						return fail(err)
					}
				}
			}
		} else if err := checkText(strings.TrimSpace(src), "input", maxRequestChars); err != nil {
			return fail(err)
		}
	}
	base := a.server
	if base == "" {
		model := a.model
		if model == "" {
			model, err = resolveModel(ctx, a.definition, a.quant, a.offline, errs)
			if err != nil {
				return fail(err)
			}
		}
		if st, e := os.Stat(model); e != nil {
			return fail(e)
		} else if !st.Mode().IsRegular() {
			return fail(fmt.Errorf("model is not a regular file: %s", model))
		}
		prefix, err := findLlama(a.llama)
		if err != nil {
			return fail(err)
		}
		fmt.Fprintln(errs, "starting llama server...")
		s, err := startServer(ctx, prefix, model, a.ctx, a.verbose, a.offline, errs)
		if err != nil {
			return fail(err)
		}
		defer s.stop()
		base = s.base
	}
	c := newClient(base)
	defer c.http.CloseIdleConnections()
	switch {
	case a.interactive:
		err = repl(ctx, c, a.opt, in, out, errs)
	case a.file != "":
		if a.output != "" {
			f, e := os.Create(a.output)
			if e != nil {
				return fail(e)
			}
			err = translateDoc(ctx, c, src, a.opt, f)
			e = f.Close()
			if err == nil {
				err = e
			}
		} else {
			err = translateDoc(ctx, c, src, a.opt, out)
		}
	case docMode:
		err = translateDoc(ctx, c, strings.TrimRight(src, "\n"), a.opt, out)
		if err == nil {
			_, err = fmt.Fprintln(out)
		}
	default:
		var tr string
		tr, err = c.translate(ctx, strings.TrimSpace(src), a.opt)
		if err == nil {
			_, err = fmt.Fprintln(out, tr)
		}
	}
	if err != nil {
		return fail(err)
	}
	return 0
}
