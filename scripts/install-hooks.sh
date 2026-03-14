#!/usr/bin/env bash
# Install git hooks from scripts/ into .git/hooks/.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
HOOKS_DIR="$REPO_ROOT/.git/hooks"

for hook in pre-commit pre-push; do
    src="$SCRIPT_DIR/$hook"
    dst="$HOOKS_DIR/$hook"
    if [ -f "$src" ]; then
        cp "$src" "$dst"
        chmod +x "$dst"
        echo "Installed $hook hook."
    fi
done

echo "Done. Git hooks installed."
