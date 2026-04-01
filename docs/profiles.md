# Fuse Profiles

Fuse uses one profile per installation to set sensible defaults for the judge
and caution fallback behavior.

## Profiles

| Profile | Summary |
|---------|---------|
| `relaxed` | Minimal friction. CAUTION logs by default. |
| `balanced` | Judge-enabled default for most users. |
| `strict` | Judge-enabled mode with tighter review on suspicious commands. |

## Behavior

- `fuse install claude` and `fuse install codex` prompt for a profile when run
  interactively.
- `fuse profile` shows the current profile and the effective settings.
- `fuse profile set <name>` updates the active profile in
  `~/.fuse/config/config.yaml`.
- The generated config scaffold documents the `llm_judge` and
  `caution_fallback` settings so users can override them if needed.

## Defaults

- Missing `profile` in an existing config maps to `relaxed`.
- Existing configs with `llm_judge.mode: active` resolve to `balanced`.
- `caution_fallback: log` keeps CAUTION as a non-blocking warning.
