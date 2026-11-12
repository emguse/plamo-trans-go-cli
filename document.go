package main

import (
	"context"
	"io"
	"strings"
)

func translateDoc(ctx context.Context, c *client, text string, o options, out io.Writer) error {
	_, err := translateDocWithHistory(ctx, c, text, o, out, nil)
	return err
}

// Return successful segment pairs so REPL context survives between inputs.
func translateDocWithHistory(ctx context.Context, c *client, text string, o options, out io.Writer, history []pair) ([]pair, error) {
	for _, s := range splitDocument(text, 2000, o.TranslateCode) {
		if s.Translatable && strings.TrimSpace(s.Text) != "" {
			tr, err := c.stream(ctx, s.Text, trimHistory(history), o, out)
			if err != nil {
				return history, err
			}
			history = append(trimHistory(history), pair{s.Text, tr})
		} else {
			if _, err := io.WriteString(out, s.Text); err != nil {
				return history, err
			}
		}
		if _, err := io.WriteString(out, s.Separator); err != nil {
			return history, err
		}
	}
	return trimHistory(history), nil
}
