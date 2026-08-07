package docmd

import (
	"regexp"
	"strings"
)

// Flattening turns anydoc's GitHub-Flavored Markdown into the plain prose that
// item.body holds everywhere else in Openmind. Body feeds FTS, embeddings, the
// AI summariser, Send-to-Kindle EPUBs (which HTML-escape each paragraph),
// highlight offsets, MCP resources and reader mode — none of which render
// Markdown, so syntax left in place would surface as literal "##" and "|---|".
//
// Paragraph breaks are preserved because reader mode and the EPUB writer both
// split body on blank lines.

var (
	// atxHeading matches "## Heading" and the closed-ATX "## Heading ##".
	atxHeading = regexp.MustCompile(`^ {0,3}(#{1,6})\s+(.*?)\s*#*\s*$`)
	// setextUnderline matches the "===" / "---" underline of a setext heading.
	setextUnderline = regexp.MustCompile(`^ {0,3}(=+|-+)\s*$`)
	// thematicBreak matches "---", "***", "___" rules of three or more.
	thematicBreak = regexp.MustCompile(`^ {0,3}((\*\s*){3,}|(-\s*){3,}|(_\s*){3,})$`)
	// listMarker matches bullet and ordered list markers, including task boxes.
	listMarker = regexp.MustCompile(`^(\s*)([-*+]|\d+[.)])\s+(\[[ xX]\]\s+)?`)
	// blockQuote matches one or more leading "> " markers.
	blockQuote = regexp.MustCompile(`^\s*(>\s?)+`)
	// tableDivider matches a GFM header separator like "|---|:--:|".
	tableDivider = regexp.MustCompile(`^\s*\|?[\s:|-]*-[\s:|-]*\|?\s*$`)
	// fence matches an opening or closing code fence.
	fence = regexp.MustCompile("^\\s*(```|~~~)")

	// image must run before link: "![alt](src)" keeps the alt text only.
	mdImage = regexp.MustCompile(`!\[([^\]]*)\]\([^)]*\)`)
	// mdLink keeps the label of "[label](href)".
	mdLink = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	// refLink keeps the label of "[label][ref]".
	refLink = regexp.MustCompile(`\[([^\]]*)\]\[[^\]]*\]`)
	// autoLink unwraps "<https://example.com>".
	autoLink = regexp.MustCompile(`<(https?://[^>\s]+)>`)
	// footnoteRef drops "[^1]" markers.
	footnoteRef = regexp.MustCompile(`\[\^[^\]]*\]`)
	// codeSpan keeps the contents of `code`.
	codeSpan = regexp.MustCompile("`+([^`]*)`+")
	// emphasis strips **bold**, *em*, __bold__, _em_ and ~~strike~~ markers,
	// applied repeatedly so nested runs unwrap.
	emphasis = regexp.MustCompile(`(\*\*|__|\*|_|~~)([^*_~]+)(\*\*|__|\*|_|~~)`)
	// mdEscapedChar matches backslash-escaped punctuation.
	mdEscapedChar = regexp.MustCompile(`\\([\\` + "`" + `*_{}\[\]()#+\-.!|>~])`)
	// blankRun collapses three or more newlines into a paragraph break.
	blankRun = regexp.MustCompile(`\n{3,}`)
	// trailingSpace strips space before a newline.
	trailingSpace = regexp.MustCompile(`[ \t]+\n`)
)

// FirstHeading returns the text of the Markdown's first level-1 ATX heading,
// or "" when there is none. Fenced code is skipped so a "# comment" inside a
// code block is never mistaken for a title.
func FirstHeading(markdown string) string {
	inFence := false
	for line := range strings.SplitSeq(markdown, "\n") {
		if fence.MatchString(line) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if m := atxHeading.FindStringSubmatch(line); m != nil && len(m[1]) == 1 {
			if title := strings.TrimSpace(stripInline(m[2])); title != "" {
				return title
			}
		}
	}
	return ""
}

// Flatten converts Markdown to plain prose: block markers are removed, tables
// become tab-separated lines, fenced code is kept verbatim, and paragraph
// breaks survive. It is a pure function, so re-running enrichment on the same
// document reproduces byte-identical output.
func Flatten(markdown string) string {
	lines := strings.Split(markdown, "\n")
	out := make([]string, 0, len(lines))
	inFence := false

	for i, line := range lines {
		if fence.MatchString(line) {
			inFence = !inFence
			continue // drop the fence itself, keep what it wraps
		}
		if inFence {
			out = append(out, line)
			continue
		}

		if thematicBreak.MatchString(line) {
			out = append(out, "")
			continue
		}
		// A setext underline belongs to the line above, which was already
		// emitted as ordinary text; drop the underline itself.
		if setextUnderline.MatchString(line) && i > 0 && strings.TrimSpace(lines[i-1]) != "" {
			continue
		}
		if tableDivider.MatchString(line) && strings.Contains(line, "-") && strings.Contains(line, "|") {
			continue
		}

		line = blockQuote.ReplaceAllString(line, "")
		if m := atxHeading.FindStringSubmatch(line); m != nil {
			line = m[2]
		}
		line = listMarker.ReplaceAllString(line, "$1")
		if isTableRow(line) {
			line = flattenTableRow(line)
		}
		out = append(out, stripInline(line))
	}

	text := strings.Join(out, "\n")
	text = trailingSpace.ReplaceAllString(text, "\n")
	text = blankRun.ReplaceAllString(text, "\n\n")
	return strings.TrimSpace(text)
}

// isTableRow reports whether line looks like a GFM table row: a leading pipe
// and at least one more.
func isTableRow(line string) bool {
	t := strings.TrimSpace(line)
	return strings.HasPrefix(t, "|") && strings.Count(t, "|") >= 2
}

// flattenTableRow renders "| a | b |" as "a\tb", so tabular content stays
// readable as prose without pipe noise reaching embeddings or an EPUB.
func flattenTableRow(line string) string {
	t := strings.TrimSpace(line)
	t = strings.TrimPrefix(t, "|")
	t = strings.TrimSuffix(t, "|")
	cells := strings.Split(t, "|")
	for i, c := range cells {
		cells[i] = strings.TrimSpace(c)
	}
	return strings.Join(cells, "\t")
}

// escapeBase starts a Unicode private-use range that backslash-escaped
// characters are parked in while inline syntax is stripped. Without it an
// escaped "\*" still reads as an emphasis marker and is eaten along with the
// text up to the next one. Escapables are all ASCII, so the original character
// is recoverable by subtracting the base.
//
// Only code points whose offset is one of the characters mdEscapedChar can
// match are ever unparked, so a private-use code point that came from the
// document itself is left alone. Restoring the whole 0x00–0x7F range would
// rewrite a literal U+E000 into a NUL byte, and Postgres rejects NUL in a text
// column — the enrichment write would then fail identically on every retry.
const escapeBase = 0xE000

// mdEscapable is exactly the set mdEscapedChar matches, and therefore the only
// bytes restoreEscapes may emit. Keep the two in step.
const mdEscapable = "\\`*_{}[]()#+-.!|>~"

// protectEscapes parks each "\x" sequence at escapeBase+x.
func protectEscapes(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	return mdEscapedChar.ReplaceAllStringFunc(s, func(m string) string {
		return string(rune(escapeBase + int(m[len(m)-1])))
	})
}

// restoreEscapes brings parked characters back as their bare selves.
func restoreEscapes(s string) string {
	if !strings.ContainsFunc(s, isParked) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if isParked(r) {
			b.WriteByte(byte(r - escapeBase))
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// isParked reports whether r holds a protected escape this package created.
func isParked(r rune) bool {
	if r < escapeBase || r >= escapeBase+0x80 {
		return false
	}
	return strings.IndexByte(mdEscapable, byte(r-escapeBase)) >= 0
}

// stripInline removes inline Markdown syntax, keeping the visible text.
func stripInline(s string) string {
	s = protectEscapes(s)
	s = mdImage.ReplaceAllString(s, "$1")
	s = mdLink.ReplaceAllString(s, "$1")
	s = refLink.ReplaceAllString(s, "$1")
	s = autoLink.ReplaceAllString(s, "$1")
	s = footnoteRef.ReplaceAllString(s, "")
	s = codeSpan.ReplaceAllString(s, "$1")
	// Emphasis nests (e.g. "**bold _and_ italic**"), and one pass only peels
	// the outermost run. Iterate to a fixed point, bounded so a pathological
	// line cannot spin.
	for range 4 {
		next := emphasis.ReplaceAllString(s, "$2")
		if next == s {
			break
		}
		s = next
	}
	s = restoreEscapes(s)
	return strings.TrimRight(s, " \t")
}
