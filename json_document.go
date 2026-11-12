package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type jsonValue struct {
	start, end int
	text       string
}

// Locate string values in validated JSON. Replacing only these spans preserves
// keys, duplicate members, number spelling, ordering, and source whitespace.
func jsonValues(src string) ([]jsonValue, error) {
	if !json.Valid([]byte(src)) {
		return nil, fmt.Errorf("invalid JSON input")
	}
	var values []jsonValue
	for i := 0; i < len(src); i++ {
		if src[i] != '"' {
			continue
		}
		start := i
		i++
		for src[i] != '"' {
			if src[i] == '\\' {
				i++
			}
			i++
		}
		end := i + 1
		next := end
		for next < len(src) && strings.ContainsRune(" \t\r\n", rune(src[next])) {
			next++
		}
		if next < len(src) && src[next] == ':' {
			continue
		}
		var value string
		if err := json.Unmarshal([]byte(src[start:end]), &value); err != nil {
			return nil, err
		}
		// Validate every value before issuing requests or opening an output file.
		if strings.Contains(value, "<|plamo:") {
			return nil, fmt.Errorf("JSON value contains reserved token '<|plamo:'")
		}
		values = append(values, jsonValue{start, end, value})
	}
	return values, nil
}

func translateInput(ctx context.Context, c *client, src string, o options, out io.Writer) error {
	if o.Format == "json" {
		return translateJSON(ctx, c, src, o, out)
	}
	return translateDoc(ctx, c, src, o, out)
}

func translateJSON(ctx context.Context, c *client, src string, o options, out io.Writer) error {
	values, err := jsonValues(src)
	if err != nil {
		return err
	}
	var result strings.Builder
	pos := 0
	for _, value := range values {
		result.WriteString(src[pos:value.start])
		translated := value.text
		if strings.TrimSpace(value.text) != "" {
			var buf strings.Builder
			// Treat values as plain text, including Markdown-looking content.
			parts := splitLong(value.text, 2000)
			for i, part := range parts {
				if i > 0 {
					buf.WriteByte(' ')
				}
				if _, err := c.stream(ctx, part, nil, o, &buf); err != nil {
					return err
				}
			}
			translated = buf.String()
		}
		encoded, err := json.Marshal(translated)
		if err != nil {
			return err
		}
		result.Write(encoded)
		pos = value.end
	}
	result.WriteString(src[pos:])
	_, err = io.WriteString(out, result.String())
	return err
}
