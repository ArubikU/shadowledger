#!/usr/bin/env bash
# Self-update a ShadowLedger node from the latest GitHub release.
#
# Decentralization note: this is OPT-IN and for YOUR OWN node only. Auto-updating
# consensus software from a single source is a central kill switch — never run
# this fleet-wide. It verifies the release SHA256 before swapping the binary.
#
# Install as a daily systemd timer (see docs/DEPLOY.md) or run manually.
set -euo pipefail

REPO="${SL_REPO:-ArubikU/shadowledger}"
ARCH="${SL_ARCH:-linux-amd64}"
BIN_DIR="${SL_BIN_DIR:-/usr/local/bin}"
SERVICE="${SL_SERVICE:-shadowledger}"

current="$("$BIN_DIR/slnode" --version 2>&1 | awk '{print $NF}' || true)"
latest="$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" \
  | grep -o '"tag_name"[[:space:]]*:[[:space:]]*"[^"]*"' | head -1 | grep -o 'v[0-9][^"]*' || true)"

if [ -z "$latest" ]; then echo "sl-update: could not resolve latest release"; exit 0; fi
if [ "$current" = "$latest" ]; then echo "sl-update: up to date ($current)"; exit 0; fi

echo "sl-update: $current -> $latest"
tmp="$(mktemp -d)"; trap 'rm -rf "$tmp"' EXIT; cd "$tmp"
base="shadowledger-${latest}-${ARCH}"
curl -fsSLO "https://github.com/$REPO/releases/download/$latest/$base.tar.gz"
curl -fsSLO "https://github.com/$REPO/releases/download/$latest/SHA256SUMS.txt"

# Verify checksum BEFORE touching the installed binary.
grep " $base.tar.gz\$" SHA256SUMS.txt | sha256sum -c -

tar xzf "$base.tar.gz"
install -m 0755 "$base/slnode" "$BIN_DIR/slnode"
install -m 0755 "$base/slctl"  "$BIN_DIR/slctl"
command -v restorecon >/dev/null 2>&1 && restorecon "$BIN_DIR/slnode" "$BIN_DIR/slctl" || true
systemctl restart "$SERVICE"
echo "sl-update: updated to $latest, restarted $SERVICE"
