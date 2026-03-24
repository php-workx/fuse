# Hero GIF: fuse blocking a dangerous command, allowing a safe one.
#
# Usage: vhs assets/vhs/hero.tape
# Output: assets/hero.gif

Output assets/hero.gif

Set Shell "bash"
Set FontSize 22
Set Width 800
Set Height 450
Set TypingSpeed 40ms
Set CursorBlink false

# Title
Type "# fuse — a local firewall for AI agent commands"
Enter
Enter
Sleep 1s

# Dangerous command — BLOCKED
Type `echo '{"tool_name":"Bash","tool_input":{"command":"rm -rf /"}}' | fuse hook evaluate`
Sleep 500ms
Enter
Sleep 2.5s
Enter

# Safe command — allowed silently
Type `echo '{"tool_name":"Bash","tool_input":{"command":"ls -la"}}' | fuse hook evaluate && echo "✓ allowed"`
Sleep 500ms
Enter
Sleep 2.5s
Enter

# Closing
Type "# Sub-millisecond. No cloud. No API keys."
Enter
Sleep 2.5s
