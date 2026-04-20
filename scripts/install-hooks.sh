#!/usr/bin/env bash
# Install git hooks from scripts/ into .git/hooks/.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
git -C "$REPO_ROOT" config --unset core.hooksPath 2>/dev/null || true

# Resolve hooks directory via Git worktree metadata.
HOOKS_DIR="$(cd "$REPO_ROOT" && git rev-parse --path-format=absolute --git-path hooks 2>/dev/null)" \
    || HOOKS_DIR="$REPO_ROOT/.git/hooks"
mkdir -p "$HOOKS_DIR"

for hook in pre-commit pre-push; do
    src="$SCRIPT_DIR/$hook"
    dst="$HOOKS_DIR/$hook"
    if [ -f "$src" ]; then
        cp "$src" "$dst"
        chmod +x "$dst"
        echo "Installed $hook hook."
    fi
done

echo "Done. Git hooks installed in $HOOKS_DIR."
