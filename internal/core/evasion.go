package core

import (
	"net/url"
	"path"
	"strings"
	"unicode/utf8"
)

// pathCollapseMaxIterations bounds the path-collapse loop on adversarial input.
const pathCollapseMaxIterations = 16

// expandAnsiCQuoting interprets Bash $'...' literals, replacing each with its
// decoded UTF-8 bytes. Other dollar-prefixed forms ($VAR, ${VAR}, $(...),
// $[...], $((...)) ) are left untouched. Unterminated literals leave the input
// unchanged.
func expandAnsiCQuoting(s string) string {
	if !strings.Contains(s, "$'") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		// Look for $'  but not $$' or escaped \$'.
		if s[i] == '$' && i+1 < len(s) && s[i+1] == '\'' {
			// Confirm preceding char is not a backslash escape.
			if i > 0 && s[i-1] == '\\' {
				b.WriteByte(s[i])
				i++
				continue
			}
			end := findAnsiCClose(s, i+2)
			if end < 0 {
				// Unterminated — leave entire input unchanged.
				return s
			}
			decoded := decodeAnsiCBody(s[i+2 : end])
			b.WriteString(decoded)
			i = end + 1
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// findAnsiCClose returns the index of the unescaped closing single quote in
// the body of an ANSI-C $'...' literal starting at start. Returns -1 if none.
func findAnsiCClose(s string, start int) int {
	for i := start; i < len(s); i++ {
		if s[i] == '\\' {
			i++ // skip escaped char (could be \', \\, etc.)
			continue
		}
		if s[i] == '\'' {
			return i
		}
	}
	return -1
}

// decodeAnsiCBody decodes the body of a $'...' literal per the Bash spec.
// Unrecognised escapes preserve the backslash + char.
func decodeAnsiCBody(body string) string {
	var b strings.Builder
	b.Grow(len(body))
	i := 0
	for i < len(body) {
		if body[i] != '\\' {
			b.WriteByte(body[i])
			i++
			continue
		}
		if i+1 >= len(body) {
			b.WriteByte('\\')
			i++
			continue
		}
		i = decodeAnsiCEscape(&b, body, i)
	}
	return b.String()
}

// simpleAnsiCEscapes maps single-char Bash ANSI-C escapes to their byte value.
var simpleAnsiCEscapes = map[byte]byte{
	'a':  0x07,
	'b':  0x08,
	'e':  0x1B,
	'E':  0x1B,
	'f':  0x0C,
	'n':  0x0A,
	'r':  0x0D,
	't':  0x09,
	'v':  0x0B,
	'\\': '\\',
	'\'': '\'',
	'"':  '"',
	'?':  '?',
}

// decodeAnsiCEscape decodes one escape sequence starting at body[i] (which is
// the backslash). Returns the new index after the consumed sequence.
func decodeAnsiCEscape(b *strings.Builder, body string, i int) int {
	ch := body[i+1]
	if v, ok := simpleAnsiCEscapes[ch]; ok {
		b.WriteByte(v)
		return i + 2
	}
	switch ch {
	case 'x':
		return decodeHexEscape(b, body, i, 2, false)
	case 'u':
		return decodeHexEscape(b, body, i, 4, true)
	case 'U':
		return decodeHexEscape(b, body, i, 8, true)
	case 'c':
		return decodeControlEscape(b, body, i)
	}
	if ch >= '0' && ch <= '7' {
		n, consumed := parseOctal(body[i+1:], 3)
		if consumed > 0 {
			b.WriteByte(byte(n & 0xFF))
			return i + 1 + consumed
		}
	}
	// Unrecognised — preserve backslash + char literally.
	b.WriteByte('\\')
	b.WriteByte(ch)
	return i + 2
}

// decodeHexEscape consumes up to maxDigits hex digits after \x / \u / \U.
// asRune controls whether the result is encoded as a rune or a single byte.
func decodeHexEscape(b *strings.Builder, body string, i, maxDigits int, asRune bool) int {
	n, consumed := parseHex(body[i+2:], maxDigits)
	if consumed == 0 {
		b.WriteByte('\\')
		b.WriteByte(body[i+1])
		return i + 2
	}
	if asRune {
		writeRune(b, rune(n))
	} else {
		b.WriteByte(byte(n))
	}
	return i + 2 + consumed
}

// decodeControlEscape consumes \cX (control char).
func decodeControlEscape(b *strings.Builder, body string, i int) int {
	if i+2 >= len(body) {
		b.WriteByte('\\')
		b.WriteByte('c')
		return i + 2
	}
	b.WriteByte(body[i+2] & 0x1F)
	return i + 3
}

// parseHex consumes up to maxDigits hex digits from s and returns the parsed
// value plus the number of digits consumed. Returns (0, 0) if no digits.
func parseHex(s string, maxDigits int) (int, int) {
	n := 0
	consumed := 0
	for consumed < maxDigits && consumed < len(s) {
		c := s[consumed]
		var d int
		switch {
		case c >= '0' && c <= '9':
			d = int(c - '0')
		case c >= 'a' && c <= 'f':
			d = int(c-'a') + 10
		case c >= 'A' && c <= 'F':
			d = int(c-'A') + 10
		default:
			return n, consumed
		}
		n = n*16 + d
		consumed++
	}
	return n, consumed
}

// parseOctal consumes up to maxDigits octal digits.
func parseOctal(s string, maxDigits int) (int, int) {
	n := 0
	consumed := 0
	for consumed < maxDigits && consumed < len(s) {
		c := s[consumed]
		if c < '0' || c > '7' {
			return n, consumed
		}
		n = n*8 + int(c-'0')
		consumed++
	}
	return n, consumed
}

// writeRune writes a rune as UTF-8 bytes. Invalid runes are replaced with U+FFFD.
func writeRune(b *strings.Builder, r rune) {
	if !utf8.ValidRune(r) {
		r = utf8.RuneError
	}
	b.WriteRune(r)
}

// decodeURLPercents finds URL-shaped tokens (scheme://...) and replaces
// percent-encoded bytes in their path with their literal forms. The query
// string is left untouched. Tokens that do not look like URLs are unchanged.
// Invalid percent triplets preserve the original token.
func decodeURLPercents(s string) string {
	if !strings.Contains(s, "%") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	tokens := splitOnWhitespacePreserving(s)
	for i, tok := range tokens {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(decodeURLToken(tok))
	}
	return b.String()
}

// splitOnWhitespacePreserving splits on runs of single ASCII spaces. The
// caller (DisplayNormalize) has already collapsed internal whitespace, so
// splitting on a single space is sufficient.
func splitOnWhitespacePreserving(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, " ")
}

// decodeURLToken decodes percents in the path portion of a URL-shaped token.
func decodeURLToken(tok string) string {
	if !looksLikeURL(tok) {
		return tok
	}
	// Find query/fragment boundary.
	queryIdx := strings.IndexAny(tok, "?#")
	prefix := tok
	suffix := ""
	if queryIdx >= 0 {
		prefix = tok[:queryIdx]
		suffix = tok[queryIdx:]
	}
	decoded, err := url.PathUnescape(prefix)
	if err != nil {
		return tok
	}
	return decoded + suffix
}

// looksLikeURL returns true if tok begins with a URL scheme.
func looksLikeURL(tok string) bool {
	colon := strings.Index(tok, "://")
	if colon <= 0 {
		return false
	}
	for i := 0; i < colon; i++ {
		if !isURLSchemeByte(tok[i]) {
			return false
		}
	}
	return true
}

func isURLSchemeByte(c byte) bool {
	switch {
	case c >= 'a' && c <= 'z':
		return true
	case c >= 'A' && c <= 'Z':
		return true
	case c >= '0' && c <= '9':
		return true
	case c == '+' || c == '-' || c == '.':
		return true
	}
	return false
}

// collapsePaths normalises absolute-path tokens by resolving `..` and `.`
// segments. Only tokens starting with `/` (Unix) or `<drive>:\` (Windows) are
// processed. Iteration is bounded to pathCollapseMaxIterations.
func collapsePaths(s string) string {
	if !strings.Contains(s, "..") {
		return s
	}
	tokens := splitOnWhitespacePreserving(s)
	for i, tok := range tokens {
		tokens[i] = collapsePathToken(tok)
	}
	return strings.Join(tokens, " ")
}

// collapsePathToken resolves `..` segments inside an absolute-path token.
func collapsePathToken(tok string) string {
	if tok == "" {
		return tok
	}
	// Unix absolute path.
	if tok[0] == '/' {
		return collapseUnixPath(tok)
	}
	// Windows absolute path: "C:\..." or "C:/...".
	if len(tok) >= 3 && isWindowsDriveLetter(tok[0]) && tok[1] == ':' && (tok[2] == '\\' || tok[2] == '/') {
		drive := tok[:2]
		rest := strings.ReplaceAll(tok[2:], "\\", "/")
		collapsed := collapseUnixPath(rest)
		// Only convert back to backslash if the original used backslash separators.
		if strings.ContainsRune(tok, '\\') {
			collapsed = strings.ReplaceAll(collapsed, "/", `\`)
		}
		return drive + collapsed
	}
	return tok
}

// collapseUnixPath bounds path.Clean iteration on adversarial input.
func collapseUnixPath(p string) string {
	prev := p
	for i := 0; i < pathCollapseMaxIterations; i++ {
		next := path.Clean(prev)
		if next == prev {
			return next
		}
		prev = next
	}
	return prev
}

func isWindowsDriveLetter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}
