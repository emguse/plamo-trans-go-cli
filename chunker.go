package main

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

type segment struct {
	Text         string
	Translatable bool
	Separator    string
}

// splitDocument preserves all source separators and fenced code, including EOF newlines.
func splitDocument(text string, maxChars int) []segment {
	var segs []segment
	var buf strings.Builder
	inCode := false
	flush := func(tr bool) {
		if buf.Len() == 0 {
			return
		}
		t := buf.String()
		sep := ""
		if strings.HasSuffix(t, "\n") {
			t = strings.TrimSuffix(t, "\n")
			sep = "\n"
		}
		segs = append(segs, segment{t, tr, sep})
		buf.Reset()
	}
	for _, raw := range strings.SplitAfter(text, "\n") {
		if raw == "" {
			continue
		}
		line := strings.TrimSuffix(raw, "\n")
		if strings.HasPrefix(strings.TrimLeftFunc(line, unicode.IsSpace), "```") {
			if !inCode {
				flush(true)
				inCode = true
				buf.WriteString(raw)
			} else {
				buf.WriteString(raw)
				inCode = false
				flush(false)
			}
		} else if inCode {
			buf.WriteString(raw)
		} else if strings.TrimSpace(line) == "" {
			flush(true)
			if len(segs) == 0 {
				segs = append(segs, segment{})
			}
			segs[len(segs)-1].Separator += raw
		} else {
			buf.WriteString(raw)
		}
	}
	flush(!inCode)
	// Reserved-token examples in inline code must never enter the model prompt.
	// Keep them verbatim, just as fenced code, before applying chunk limits.
	var protected []segment
	for _, s := range segs {
		if !s.Translatable {
			protected = append(protected, s)
			continue
		}
		parts := protectInlineTokens(s.Text)
		parts[len(parts)-1].Separator += s.Separator
		protected = append(protected, parts...)
	}
	var out []segment
	for _, s := range protected {
		if s.Translatable && utf8.RuneCountInString(s.Text) > maxChars {
			parts := splitLong(s.Text, maxChars)
			for i, p := range parts {
				sep := " "
				if i == len(parts)-1 {
					sep = s.Separator
				}
				out = append(out, segment{p, true, sep})
			}
		} else {
			out = append(out, s)
		}
	}
	return out
}
func hardSplit(text string, maxChars int) []string {
	r := []rune(text)
	var out []string
	for len(r) > maxChars {
		cut := maxChars
		for i := maxChars - 1; i >= 0; i-- {
			if r[i] == ' ' {
				cut = i + 1
				break
			}
		}
		out = append(out, string(r[:cut]))
		r = r[cut:]
	}
	return append(out, string(r))
}
func splitLong(text string, maxChars int) []string {
	var sentences []string
	r := []rune(text)
	start := 0
	for i := 0; i < len(r); i++ {
		if strings.ContainsRune("。！？.!?", r[i]) {
			i++
			for i < len(r) && unicode.IsSpace(r[i]) {
				i++
			}
			sentences = append(sentences, string(r[start:i]))
			start = i
			i--
		}
	}
	sentences = append(sentences, string(r[start:]))
	var parts []string
	cur := ""
	for _, s := range sentences {
		for _, p := range hardSplit(s, maxChars) {
			if cur != "" && utf8.RuneCountInString(cur)+utf8.RuneCountInString(p) > maxChars {
				parts = append(parts, cur)
				cur = ""
			}
			cur += p
		}
	}
	if cur != "" {
		parts = append(parts, cur)
	}
	if len(parts) == 0 {
		return []string{text}
	}
	return parts
}
func trimHistory(h []pair) []pair {
	if len(h) > 4 {
		h = h[len(h)-4:]
	}
	n := 0
	for _, p := range h {
		n += utf8.RuneCountInString(p.Source) + utf8.RuneCountInString(p.Translation)
	}
	for len(h) > 0 && n > 3000 {
		n -= utf8.RuneCountInString(h[0].Source) + utf8.RuneCountInString(h[0].Translation)
		h = h[1:]
	}
	return h
}

// protectInlineTokens recognizes matching Markdown backtick runs, including
// multi-backtick delimiters. Ordinary prose remains subject to token validation.
func protectInlineTokens(text string) []segment {
	var out []segment
	appendProse := func(s string) {
		if s == "" {
			return
		}
		left := strings.TrimLeftFunc(s, unicode.IsSpace)
		prefix := s[:len(s)-len(left)]
		body := strings.TrimRightFunc(left, unicode.IsSpace)
		suffix := left[len(body):]
		if prefix != "" {
			out = append(out, segment{Text: prefix})
		}
		if body != "" {
			out = append(out, segment{body, true, suffix})
		}
	}
	start := 0
	for i := 0; i < len(text); {
		if text[i] != '`' {
			i++
			continue
		}
		slashes := 0
		for j := i - 1; j >= 0 && text[j] == '\\'; j-- {
			slashes++
		}
		if slashes%2 == 1 {
			i++
			continue
		}
		open := i
		for i < len(text) && text[i] == '`' {
			i++
		}
		width := i - open
		closeStart, closeEnd := -1, -1
		for j := i; j < len(text); {
			if text[j] != '`' {
				j++
				continue
			}
			end := j
			for end < len(text) && text[end] == '`' {
				end++
			}
			if end-j == width {
				closeStart, closeEnd = j, end
				break
			}
			j = end
		}
		if closeStart < 0 {
			continue
		}
		if strings.Contains(text[i:closeStart], "<|plamo:") {
			appendProse(text[start:open])
			out = append(out, segment{Text: text[open:closeEnd]})
			start = closeEnd
		}
		i = closeEnd
	}
	if len(out) == 0 {
		return []segment{{Text: text, Translatable: true}}
	}
	appendProse(text[start:])
	return out
}
