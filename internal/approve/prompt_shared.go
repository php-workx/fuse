package approve

import (
	"fmt"
	"strings"

	"github.com/php-workx/fuse/internal/sanitize"
)

var errNonInteractive = fmt.Errorf("fuse:NON_INTERACTIVE_MODE STOP. Approval requires an interactive terminal (/dev/tty unavailable)")

var errPromptTimeout = fmt.Errorf("fuse:TIMEOUT_WAITING_FOR_USER STOP. The user did not approve this action in time")

// sanitizePrompt delegates to the shared sanitize package and additionally
// replaces \n and \r with spaces to prevent prompt layout injection.
func sanitizePrompt(s string) string {
	s = sanitize.String(s)
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	return s
}
