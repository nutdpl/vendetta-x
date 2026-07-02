package main

import "strings"

// quoteCap is the most quoted lines a reply carries; a novel-length original
// gets elided rather than drowning the reply under it.
const quoteCap = 20

// quoteLines renders an original message as the classic >-quoted block a
// reply composer opens with: "Handle> the line", word-wrapped to width,
// capped at quoteCap lines with an elision marker, and a trailing blank line
// where the cursor lands.
func quoteLines(from, body string, width int) []string {
	prefix := from + "> "
	avail := width - len(prefix)
	if avail < 10 {
		avail = 10
	}

	var out []string
	for _, raw := range strings.Split(body, "\n") {
		raw = strings.TrimRight(raw, " \t")
		// Preserve deliberate blank lines (paragraph breaks) as bare quotes.
		if strings.TrimSpace(raw) == "" {
			out = append(out, strings.TrimRight(prefix, " "))
			continue
		}
		// Nested quotes collapse: "a> b> text" noise gets one more level max.
		line := raw
		for len(line) > 0 {
			if len(line) <= avail {
				out = append(out, prefix+line)
				break
			}
			// Break at the last space that fits, or hard-break a long word.
			cut := strings.LastIndex(line[:avail+1], " ")
			if cut <= 0 {
				cut = avail
			}
			out = append(out, prefix+strings.TrimRight(line[:cut], " "))
			line = strings.TrimLeft(line[cut:], " ")
		}
		if len(out) > quoteCap {
			break
		}
	}

	// Drop trailing bare-quote lines so the block ends on content.
	for len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == strings.TrimSpace(strings.TrimRight(prefix, " ")) {
		out = out[:len(out)-1]
	}
	if len(out) > quoteCap {
		out = append(out[:quoteCap], from+"> [...]")
	}
	if len(out) == 0 {
		return nil
	}
	return append(out, "")
}
