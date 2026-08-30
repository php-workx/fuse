#!/usr/bin/env bash
set -euo pipefail

if [[ "$(uname -s)" != "Darwin" ]]; then
    echo "Fuse Notifier.app can only be built on macOS" >&2
    exit 1
fi

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
source_dir="$root/native/macos/FuseNotifier"
destination_root=${1:?usage: scripts/build-macos-notifier.sh DESTINATION_DIRECTORY}
bundle="$destination_root/Fuse Notifier.app"
swift build --package-path "$source_dir" -c release --arch arm64 --arch x86_64
binary_dir=$(swift build --package-path "$source_dir" -c release --arch arm64 --arch x86_64 --show-bin-path)

rm -rf "$bundle"
mkdir -p "$bundle/Contents/MacOS" "$bundle/Contents/Resources"
cp -f "$binary_dir/FuseNotifier" "$bundle/Contents/MacOS/FuseNotifier"
cp -f "$source_dir/Resources/Info.plist" "$bundle/Contents/Info.plist"
cp -f "$source_dir/Resources/FuseNotifier.entitlements" "$bundle/Contents/Resources/FuseNotifier.entitlements"

codesign --force --sign - --entitlements "$source_dir/Resources/FuseNotifier.entitlements" "$bundle"
codesign --verify --strict --verbose=2 "$bundle"
printf '%s\n' "$bundle"
