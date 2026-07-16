#!/usr/bin/env sh
# Relayent bridge installer.
#
# Primary author: Navjyot Nishant
# Created on: 2026-07-16
# Description: Installs the relayent-bridge binary to ~/.local/bin and runs the
#   interactive pairing wizard. Builds from source when a Go toolchain is present,
#   otherwise downloads a release asset. POSIX sh, no dependencies beyond curl.
#
# Usage:
#   ./install.sh                      # from a checkout
#   curl -fsSL <url>/install.sh | sh  # once releases are published
#
# SECURITY NOTE ON `curl | sh`:
#   Piping a remote script into a shell means executing whatever that URL returns.
#   That is a real supply-chain risk and you should not do it blindly — for this
#   or any other tool. Read the script first:
#     curl -fsSL <url>/install.sh -o install.sh && less install.sh && sh install.sh
#   This installer never uses sudo, never writes outside your home directory, and
#   never transmits anything.

set -eu

REPO="navjyotnishant/relayent"
BIN_NAME="relayent-bridge"
INSTALL_DIR="${RELAYENT_INSTALL_DIR:-$HOME/.local/bin}"
VERSION="${RELAYENT_VERSION:-latest}"

say()  { printf '  %s\n' "$*"; }
ok()   { printf '  \033[32m✓\033[0m %s\n' "$*"; }
warn() { printf '  \033[33m!\033[0m %s\n' "$*"; }
die()  { printf '  \033[31m✗\033[0m %s\n' "$*" >&2; exit 1; }

printf '\n  Relayent bridge installer\n  ─────────────────────────\n\n'

# --- platform detection -------------------------------------------------------
os="$(uname -s)"
arch="$(uname -m)"
case "$os" in
  Darwin) goos="darwin" ;;
  Linux)  goos="linux"  ;;
  *) die "Unsupported OS: $os. Relayent's bridge supports macOS and Linux." ;;
esac
case "$arch" in
  x86_64|amd64) goarch="amd64" ;;
  arm64|aarch64) goarch="arm64" ;;
  *) die "Unsupported architecture: $arch" ;;
esac
ok "Platform: $goos/$goarch"

mkdir -p "$INSTALL_DIR"
target="$INSTALL_DIR/$BIN_NAME"

# --- obtain the binary --------------------------------------------------------
# Prefer building from the local checkout: it is the most trustworthy path
# (you can read the source you are running) and needs no network.
script_dir="$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)"
if [ -f "$script_dir/go.mod" ] && command -v go >/dev/null 2>&1; then
  say "Building from source (Go $(go version | awk '{print $3}' | sed 's/go//'))…"
  ( cd "$script_dir" && CGO_ENABLED=0 go build -ldflags "-s -w" -o "$target" ./bridge ) \
    || die "Build failed."
  ok "Built $target"
else
  command -v curl >/dev/null 2>&1 || die "curl is required to download the release."
  if [ "$VERSION" = "latest" ]; then
    url="https://github.com/$REPO/releases/latest/download/${BIN_NAME}_${goos}_${goarch}"
  else
    url="https://github.com/$REPO/releases/download/$VERSION/${BIN_NAME}_${goos}_${goarch}"
  fi
  say "Downloading $url"
  tmp="$(mktemp)"
  curl -fsSL "$url" -o "$tmp" || {
    rm -f "$tmp"
    die "Download failed. No published release yet? Clone the repo and re-run this
    script from the checkout to build from source instead."
  }
  mv "$tmp" "$target"
  ok "Downloaded $target"
fi
chmod 755 "$target"

# --- PATH ---------------------------------------------------------------------
case ":$PATH:" in
  *":$INSTALL_DIR:"*) ok "$INSTALL_DIR is on your PATH" ;;
  *)
    warn "$INSTALL_DIR is not on your PATH. Add this to your shell profile:"
    printf '\n      export PATH="%s:$PATH"\n\n' "$INSTALL_DIR"
    ;;
esac

# --- backends -----------------------------------------------------------------
say ""
say "Checking for AI CLIs on this machine:"
found=0
for cli in claude codex cursor-agent gemini; do
  if command -v "$cli" >/dev/null 2>&1; then
    ok "$cli found"
    found=$((found + 1))
  else
    say "  · $cli not installed"
  fi
done
if [ "$found" -eq 0 ]; then
  printf '\n'
  warn "No AI CLIs found. The bridge needs at least one installed AND logged in:"
  say "    Claude Code:  https://claude.com/claude-code"
  say "    Codex:        https://developers.openai.com/codex"
  say "    Cursor:       https://cursor.com/cli"
  say ""
  say "  Install and sign in to one, then run: $BIN_NAME setup"
  exit 0
fi

# --- pair ---------------------------------------------------------------------
printf '\n'
say "Starting the pairing wizard…"
exec "$target" setup
