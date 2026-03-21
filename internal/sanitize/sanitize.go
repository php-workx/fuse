// Package sanitize provides terminal-safe string sanitization for fuse.
// It strips ANSI escape sequences, control characters, and Unicode C1 codes
// to prevent terminal injection via crafted command strings or event data.
package sanitize

import "regexp"

// reControlChars matches ANSI/terminal escape sequences and non-printable control characters.
// Covers: 7-bit CSI (ESC[), BEL/ST-terminated OSC, other ESC sequences, and C0 controls.
var reControlChars = regexp.MustCompile(
	`\x1b\[[0-9;]*[a-zA-Z]` + // 7-bit CSI sequences (ESC[...letter)
		`|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)` + // OSC sequences (BEL or ST terminated)
		`|\x1b[^[\]]` + // other ESC sequences (ESC + single char)
		`|[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]`, // C0 control chars (preserve \t \n \r)
)

// String strips ANSI escape sequences, C0 controls, and Unicode C1 control
// characters (U+0080-U+009F) from a string. C1 codes are stripped at the rune
// level because some terminals interpret U+009B as CSI (same as ESC[).
func String(s string) string {
	s = reControlChars.ReplaceAllString(s, "")
	clean := make([]rune, 0, len(s))
	for _, r := range s {
		if r >= 0x80 && r <= 0x9F {
			continue
		}
		clean = append(clean, r)
	}
	return string(clean)
}
