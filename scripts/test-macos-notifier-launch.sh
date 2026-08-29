#!/usr/bin/env bash
# Stage 0 lifecycle gate. It first records the direct child diagnostic, then proves
# the prescribed LaunchServices + authenticated Unix-socket transport.
set -euo pipefail

interactive_click=false
validate_app=false
case ${1:-} in
  "")
    ;;
  --interactive-click)
    interactive_click=true
    ;;
  --validate-app)
    validate_app=true
    ;;
  *)
    printf 'usage: %s [--interactive-click|--validate-app]\n' "$0" >&2
    exit 64
    ;;
esac
[[ $# -le 1 ]] || { printf 'usage: %s [--interactive-click|--validate-app]\n' "$0" >&2; exit 64; }

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
package_dir="$repo_root/native/macos/FuseNotifier"
temporary_dir=$(mktemp -d "${TMPDIR:-/tmp}/fuse-notifier.XXXXXX")
app_bundle="$temporary_dir/Fuse Notifier.app"
socket_dir="$temporary_dir/socket"
socket_path="$socket_dir/notifier.sock"
session_token=$(openssl rand -hex 32)
notifier_pid=
cleanup() {
  if [[ -n $notifier_pid ]] && kill -0 "$notifier_pid" 2>/dev/null; then
    kill -TERM "$notifier_pid" 2>/dev/null || true
    wait "$notifier_pid" 2>/dev/null || true
  fi
  rm -rf "$temporary_dir"
}
trap cleanup EXIT

stable_bundle=${FUSE_NOTIFIER_APP:-}
require_accepted_post=false
if [[ -n $stable_bundle ]]; then
  [[ -d $stable_bundle ]] || { printf 'FUSE_NOTIFIER_APP is not a bundle directory\n' >&2; exit 64; }
  app_bundle=$(cd "$stable_bundle" && pwd -P)
  info_plist="$app_bundle/Contents/Info.plist"
  executable="$app_bundle/Contents/MacOS/FuseNotifier"
  [[ -f $info_plist && -x $executable ]] || { printf 'FUSE_NOTIFIER_APP is missing the expected executable or Info.plist\n' >&2; exit 64; }
  plutil -lint "$info_plist" >/dev/null
  [[ $(/usr/libexec/PlistBuddy -c 'Print :CFBundleIdentifier' "$info_plist") == 'com.php-workx.fuse.notifier' ]] || {
    printf 'FUSE_NOTIFIER_APP has an unexpected bundle identifier\n' >&2
    exit 64
  }
  [[ $(/usr/libexec/PlistBuddy -c 'Print :CFBundleExecutable' "$info_plist") == 'FuseNotifier' ]] || {
    printf 'FUSE_NOTIFIER_APP has an unexpected executable declaration\n' >&2
    exit 64
  }
  codesign --verify --strict --verbose=2 "$app_bundle"
  require_accepted_post=true
  if "$validate_app"; then
    printf 'validated stable notifier bundle: %s\n' "$app_bundle"
    exit 0
  fi
else
  "$validate_app" && { printf -- '--validate-app requires FUSE_NOTIFIER_APP\n' >&2; exit 64; }
  swift build --package-path "$package_dir" --product FuseNotifier -c release
  binary_dir=$(swift build --package-path "$package_dir" -c release --show-bin-path)
  mkdir -p "$app_bundle/Contents/MacOS"
  cp -f "$package_dir/Resources/Info.plist" "$app_bundle/Contents/Info.plist"
  cp -f "$binary_dir/FuseNotifier" "$app_bundle/Contents/MacOS/FuseNotifier"
  plutil -lint "$app_bundle/Contents/Info.plist" >/dev/null
  codesign --force --sign - --options runtime --entitlements "$package_dir/Resources/FuseNotifier.entitlements" "$app_bundle" >/dev/null
  codesign --verify --strict --verbose=2 "$app_bundle"
fi

mkdir -p "$socket_dir"
chmod 700 "$socket_dir"

if [[ -z $stable_bundle ]]; then
  # This current-host diagnostic deliberately keeps direct inherited pipes observable.
  # It is not selected after UNErrorDomain code 1 demonstrates the direct app cannot
  # obtain notification authorization on this host.
  coproc DIRECT_NOTIFIER { exec "$app_bundle/Contents/MacOS/FuseNotifier"; }
  notifier_pid=$DIRECT_NOTIFIER_PID
  direct_input=${DIRECT_NOTIFIER[1]}
  direct_output=${DIRECT_NOTIFIER[0]}
  printf '%s\n' '{"version":1,"type":"hello","request_id":1,"client_version":"stage0-direct-diagnostic","terminal":{"terminal_program":"Apple_Terminal","terminal_program_version":"2.14","iterm_session_id":null,"terminal_tty":"/dev/ttys001","ghostty_title_marker":null,"exact_targeting_degraded":false}}' >&"$direct_input"
  IFS= read -r -t 10 direct_ready <&"$direct_output"
  [[ $direct_ready == *'"type":"ready"'* ]] || { printf 'direct diagnostic did not become ready\n' >&2; exit 1; }
  printf '%s\n' '{"version":1,"type":"post","request_id":2,"id":"stage0-direct-diagnostic","title":"Fuse approval required","body":"Open Fuse to review this request.","sound":false}' >&"$direct_input"
  IFS= read -r -t 10 direct_accepted <&"$direct_output"
  [[ $direct_accepted == *'"type":"result"'* && $direct_accepted == *'"status":"accepted"'* && $direct_accepted == *'"request_id":2'* ]] || {
    printf 'direct diagnostic did not acknowledge desired notification state: %s\n' "$direct_accepted" >&2
    exit 1
  }
  IFS= read -r -t 10 direct_delivery <&"$direct_output"
  if [[ $direct_delivery == *'"type":"posted"'* ]]; then
    printf 'direct diagnostic delivered notification after accepted desired-state acknowledgement\n' >&2
  elif [[ $direct_delivery == *'"code":"notification_post_failed"'* ]]; then
    printf 'direct diagnostic observed unavailable notification authorization after accepted desired-state acknowledgement\n' >&2
  else
    printf 'direct diagnostic produced an unexpected delivery outcome: %s\n' "$direct_delivery" >&2
    exit 1
  fi
  eval "exec ${direct_input}>&-"
  IFS= read -r -t 10 direct_cleanup <&"$direct_output"
  [[ $direct_cleanup == *'"type":"shutdown_complete"'* ]] || { printf 'direct diagnostic did not clean up\n' >&2; exit 1; }
  wait "$notifier_pid"
  notifier_pid=
fi

# The Python server owns the mode-0600 socket, permits exactly one same-UID peer,
# and authenticates the first frame with a 256-bit random token before JSON Lines.
python3 - "$app_bundle" "$socket_path" "$session_token" "$interactive_click" "$require_accepted_post" <<'PY'
import ctypes
import hmac
import os
import socket
import subprocess
import sys

app_bundle, socket_path, token, interactive, require_accepted = sys.argv[1:]
if os.stat(os.path.dirname(socket_path)).st_mode & 0o777 != 0o700:
    raise SystemExit("socket parent directory permissions are not 0700")
server = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
server.bind(socket_path)
os.chmod(socket_path, 0o600)
if os.stat(socket_path).st_mode & 0o777 != 0o600:
    raise SystemExit("socket permissions are not 0600")
server.listen(1)
server.settimeout(10)
subprocess.run([
    "/usr/bin/open", "-n", "-g", app_bundle, "--args",
    "--connect", socket_path, "--token", token,
], check=True)
connection, _ = server.accept()
try:
    uid = ctypes.c_uint()
    gid = ctypes.c_uint()
    if ctypes.CDLL(None).getpeereid(connection.fileno(), ctypes.byref(uid), ctypes.byref(gid)) != 0:
        raise SystemExit("could not inspect Unix-socket peer UID")
    if uid.value != os.geteuid():
        raise SystemExit("socket peer UID did not match the launcher")
    connection.settimeout(10)

    def read_line(limit=32768):
        data = bytearray()
        while True:
            byte = connection.recv(1)
            if not byte:
                raise SystemExit("notifier closed the socket unexpectedly")
            if byte == b"\n":
                return bytes(data)
            data.extend(byte)
            if len(data) > limit:
                raise SystemExit("notifier exceeded the JSON-line limit")

    handshake = read_line(80)
    if not hmac.compare_digest(handshake, ("token " + token).encode()):
        raise SystemExit("notifier socket handshake token did not match")
    connection.sendall(b"ok\n")

    def request(line, expected):
        connection.sendall(line + b"\n")
        response = read_line()
        if expected not in response:
            raise SystemExit("unexpected notifier response: " + response.decode("utf-8", "replace"))
        return response

    request(b'{"version":1,"type":"hello","request_id":1,"client_version":"stage0-fallback-smoke","terminal":{"terminal_program":"Apple_Terminal","terminal_program_version":"2.14","iterm_session_id":null,"terminal_tty":"/dev/ttys001","ghostty_title_marker":null,"exact_targeting_degraded":false}}', b'"type":"ready"')
    post_accepted = request(b'{"version":1,"type":"post","request_id":2,"id":"stage0-fallback-smoke","title":"Fuse approval required","body":"Open Fuse to review this request.","sound":false}', b'"status":"accepted"')
    if b'"type":"result"' not in post_accepted:
        raise SystemExit("post did not acknowledge desired notification state: " + post_accepted.decode("utf-8", "replace"))
    post_delivery = read_line()
    notifications_posted = b'"type":"posted"' in post_delivery
    if not notifications_posted and b'"code":"notification_post_failed"' not in post_delivery:
        raise SystemExit("unexpected post delivery outcome: " + post_delivery.decode("utf-8", "replace"))

    if notifications_posted and interactive == "true":
        print("Manual click check: click the visible generic “Fuse approval required” notification now.", file=sys.stderr)
        connection.settimeout(60)
        try:
            response = read_line()
            if b'"type":"activated"' in response:
                print("Manual click check passed: the LaunchServices child emitted activated.")
            else:
                print("Manual click check did not produce activated; this is not simulated evidence.", file=sys.stderr)
        except TimeoutError:
            print("Manual click check timed out; this is not simulated evidence.", file=sys.stderr)
        connection.settimeout(10)
    elif notifications_posted:
        print("Manual click proof is not automated. Re-run with --interactive-click to exercise the real delegate.", file=sys.stderr)
    else:
        print("Notification authorization is unavailable on this host; post was acknowledged with notification_post_failed.", file=sys.stderr)

    connection.shutdown(socket.SHUT_WR)
    if b'"type":"shutdown_complete"' not in read_line():
        raise SystemExit("notifier did not confirm EOF cleanup")
    if connection.recv(1) != b"":
        raise SystemExit("notifier did not exit after EOF cleanup")
finally:
    connection.close()
    server.close()

if notifications_posted:
    print("LaunchServices socket launch, post acknowledgement, and EOF cleanup passed")
elif require_accepted == "true":
    raise SystemExit("stable notifier bundle did not receive an accepted post")
else:
    print("LaunchServices socket transport and EOF cleanup passed; notification authorization remains an external gate", file=sys.stderr)
    raise SystemExit(2)
PY
