package adapters

import "unicode/utf8"

// maxAgentTextBytes caps input-derived text (classification reasons, file
// paths, tool names) embedded in messages the agent reads. Agents keep hook
// denials in context and replay them on every later turn, so a message must not
// grow with attacker-controlled input.
const maxAgentTextBytes = 256

const agentTextTruncationMarker = "…[truncated]"

// boundForAgent truncates s to at most maxAgentTextBytes bytes on a UTF-8
// boundary and marks the cut.
func boundForAgent(s string) string {
	if len(s) <= maxAgentTextBytes {
		return s
	}
	cut := maxAgentTextBytes - len(agentTextTruncationMarker)
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + agentTextTruncationMarker
}
