package hakkacode

import "regexp"

// Table cells are rendered outside glamour (see renderTable), so simple
// inline markdown — bold, italic, inline code — would otherwise show up
// as literal asterisks/backticks. This is a small, regex-based inline
// formatter for exactly that: not a general markdown parser, just enough
// to make table cells readable.
var (
	inlineCodeRE   = regexp.MustCompile("`([^`]+)`")
	inlineBoldRE   = regexp.MustCompile(`\*\*([^*]+)\*\*|__([^_]+)__`)
	inlineItalicRE = regexp.MustCompile(`\*([^*]+)\*|_([^_]+)_`)
)

// renderInline applies bold/italic/code styling to a single line of
// plain text. Order matters: code first (so its contents are protected
// from further substitution), then bold (the longer marker), then
// italic — otherwise "**bold**" would get its outer "*" pairs partially
// consumed by the italic pass first.
func renderInline(s string) string {
	s = inlineCodeRE.ReplaceAllString(s, sgrCyan+"$1"+sgrReset)
	s = inlineBoldRE.ReplaceAllStringFunc(s, func(match string) string {
		sub := inlineBoldRE.FindStringSubmatch(match)
		text := sub[1]
		if text == "" {
			text = sub[2]
		}
		return sgrBold + text + sgrReset
	})
	s = inlineItalicRE.ReplaceAllStringFunc(s, func(match string) string {
		sub := inlineItalicRE.FindStringSubmatch(match)
		text := sub[1]
		if text == "" {
			text = sub[2]
		}
		return sgrItalic + text + sgrReset
	})
	return s
}
