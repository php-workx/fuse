#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
temporary_dir=$(mktemp -d "${TMPDIR:-/tmp}/fuse-notifier-selection.XXXXXX")
fixture="$temporary_dir/Fuse Notifier.app"
trap 'rm -rf "$temporary_dir"' EXIT

mkdir -p "$fixture/Contents/MacOS"
cat >"$fixture/Contents/Info.plist" <<'PLIST'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>CFBundleIdentifier</key><string>com.php-workx.fuse.notifier</string>
<key>CFBundleExecutable</key><string>FuseNotifier</string>
</dict></plist>
PLIST
printf '#!/usr/bin/env bash\nexit 0\n' >"$fixture/Contents/MacOS/FuseNotifier"
chmod 755 "$fixture/Contents/MacOS/FuseNotifier"
codesign --force --sign - "$fixture" >/dev/null

FUSE_NOTIFIER_APP="$fixture" "$repo_root/scripts/test-macos-notifier-launch.sh" --validate-app
