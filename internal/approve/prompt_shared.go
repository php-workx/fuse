package approve

import (
	"fmt"
	"os"
	"strings"

	"github.com/php-workx/fuse/internal/sanitize"
)

var errNonInteractive = fmt.Errorf("fuse:NON_INTERACTIVE_MODE STOP. Approval requires an interactive terminal (console unavailable)")

var errPromptTimeout = fmt.Errorf("fuse:TIMEOUT_WAITING_FOR_USER STOP. The user did not approve this action in time")

// sanitizePrompt delegates to the shared sanitize package and additionally
// replaces \n and \r with spaces to prevent prompt layout injection.
func sanitizePrompt(s string) string {
	s = sanitize.String(s)
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	return s
}

// getContextVars returns relevant environment variables for the prompt.
// Used by both Unix and Windows prompt implementations.
func getContextVars() string {
	relevantVars := []string{
		"AWS_PROFILE", "AWS_REGION", "AWS_DEFAULT_REGION",
		"TF_WORKSPACE", "TF_VAR_environment",
		"KUBECONFIG", "KUBECONTEXT",
		"GCP_PROJECT", "GOOGLE_CLOUD_PROJECT",
		"AZURE_SUBSCRIPTION",
	}

	var result string
	for _, v := range relevantVars {
		val := os.Getenv(v)
		if val != "" {
			if result != "" {
				result += ", "
			}
			result += v + "=" + sanitize.String(val)
		}
	}
	return result
}
