#!/usr/bin/env bash
set -euo pipefail

current_root="$(git rev-parse --show-toplevel 2>/dev/null)" || {
    echo "error: run this script inside a git worktree" >&2
    exit 1
}

common_dir="$(git rev-parse --path-format=absolute --git-common-dir 2>/dev/null)" || {
    echo "error: unable to resolve the shared git directory" >&2
    exit 1
}

primary_root="$(cd "$common_dir/.." && pwd -P)"
current_root="$(cd "$current_root" && pwd -P)"
info_exclude_path="$(git rev-parse --git-path info/exclude)"

if [ "$current_root" = "$primary_root" ]; then
    echo "error: already in the primary checkout: $current_root" >&2
    exit 1
fi

ensure_excluded() {
    local path="$1"

    if [ -f "$info_exclude_path" ]; then
        if ! grep -Fxq "$path" "$info_exclude_path"; then
            printf '%s\n' "$path" >> "$info_exclude_path"
        fi
        return
    fi

    mkdir -p "$(dirname "$info_exclude_path")"
    printf '%s\n' "$path" > "$info_exclude_path"
}

ensure_link() {
    local name="$1"
    local source_path="$primary_root/$name"
    local target_path="$current_root/$name"

    if [ ! -e "$source_path" ]; then
        echo "error: shared path is missing in the primary checkout: $source_path" >&2
        exit 1
    fi

    if [ -L "$target_path" ]; then
        local existing_target
        existing_target="$(readlink "$target_path")"
        if [ "$existing_target" = "$source_path" ]; then
            echo "$name already linked"
            return
        fi
        rm -f "$target_path"
    elif [ -e "$target_path" ]; then
        if [ "$name" = ".tickets" ]; then
            local status
            status="$(git status --porcelain=v1 --untracked-files=normal -- "$name")"
            if [ -n "$status" ]; then
                echo "error: refusing to replace $target_path with local changes:" >&2
                echo "$status" >&2
                exit 1
            fi
        elif find "$target_path" -mindepth 1 -maxdepth 1 -print -quit | grep -q .; then
            echo "error: refusing to replace non-empty path: $target_path" >&2
            exit 1
        fi

        rm -rf "$target_path"
    fi

    ln -s "$source_path" "$target_path"
    echo "linked $target_path -> $source_path"
    ensure_excluded "$name"

    if [ "$name" = ".tickets" ]; then
        if git ls-files -- "$name" | grep -q .; then
            git ls-files -z -- "$name" | xargs -0 git update-index --skip-worktree --
        fi
    fi
}

ensure_link ".agents"
ensure_link ".tickets"
