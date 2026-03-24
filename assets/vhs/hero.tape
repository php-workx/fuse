# Hero GIF: fuse blocking a dangerous command, allowing a safe one.
# Records the core value proposition in ~8 seconds.
#
# Usage: vhs assets/vhs/hero.tape
# Output: assets/hero.gif

Output assets/hero.gif

Set Shell "bash"
Set FontSize 16
Set Width 900
Set Height 500
Set Padding 20
Set Theme "Catppuccin Mocha"
Set TypingSpeed 40ms
Set CursorBlink false

# Title
Type "# fuse — a local firewall for AI agent commands"
Enter
Sleep 1s

# Dangerous command — BLOCKED
Type `echo '{"tool_name":"Bash","tool_input":{"command":"rm -rf /"},"session_id":"demo","cwd":"/tmp"}' | fuse hook evaluate`
Sleep 500ms
Enter
Sleep 2s

# Safe command — allowed silently
Type `echo '{"tool_name":"Bash","tool_input":{"command":"ls -la"},"session_id":"demo","cwd":"/tmp"}' | fuse hook evaluate && echo "✓ allowed"`
Sleep 500ms
Enter
Sleep 2s

# Show classification speed
Type "# Classification: sub-millisecond. No cloud. No API keys."
Enter
Sleep 2s
