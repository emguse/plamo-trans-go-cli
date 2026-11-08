package main

import (
	"context"
	"io"
	"strings"
)

func translateDoc(ctx context.Context, c *client, text string, o options, out io.Writer) error {
	var history []pair
	for _, s := range splitDocument(text, 2000) {
		if s.Translatable && strings.TrimSpace(s.Text) != "" {
			tr, err := c.stream(ctx, s.Text, trimHistory(history), o, out)
			if err != nil {
				return err
			}
			history = append(trimHistory(history), pair{s.Text, tr})
		} else {
			if _, err := io.WriteString(out, s.Text); err != nil {
				return err
			}
		}
		if _, err := io.WriteString(out, s.Separator); err != nil {
			return err
		}
	}
	return nil
}
