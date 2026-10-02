#!/bin/sh
# VaultChron installer for Linux and macOS.
#
# Environment variables:
#   VAULTCHRON_VERSION      Target release version (e.g. v0.1.0). If unset,
#                           the latest published release is detected.
#   VAULTCHRON_INSTALL_DIR  Target directory for installed binaries.
#                           Defaults to $HOME/.local/bin.
#   VAULTCHRON_BASE_URL     Base URL for release downloads.
#                           Defaults to https://github.com/ZeezyCodes/vaultchron/releases/download/<version>.
set -eu

REPO="ZeezyCodes/vaultchron"
TMP_DIR=""

cleanup() {
  if [ -n "$TMP_DIR" ]; then
    rm -rf "$TMP_DIR"
  fi
}

trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

main() {
  command -v curl >/dev/null || { echo "error: curl is required but not installed" >&2; exit 1; }
  command -v tar >/dev/null || { echo "error: tar is required but not installed" >&2; exit 1; }

  if command -v sha256sum >/dev/null; then
    SHACMD="sha256sum"
  elif command -v shasum >/dev/null; then
    SHACMD="shasum"
  else
    echo "error: sha256sum or shasum is required for checksum verification" >&2
    exit 1
  fi

  OS_RAW="$(uname -s)"
  case "$OS_RAW" in
    Linux) OS="linux" ;;
    Darwin) OS="darwin" ;;
    *)
      echo "error: unsupported operating system: $OS_RAW (Linux and Darwin/macOS supported)" >&2
      exit 1
      ;;
  esac

  ARCH_RAW="$(uname -m)"
  case "$ARCH_RAW" in
    x86_64|amd64) ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *)
      echo "error: unsupported architecture: $ARCH_RAW (amd64 and arm64 supported)" >&2
      exit 1
      ;;
  esac

  if [ -n "${VAULTCHRON_VERSION:-}" ]; then
    VERSION="$VAULTCHRON_VERSION"
    case "$VERSION" in
      v[0-9]*.[0-9]*.[0-9]*) ;;
      *)
        echo "error: invalid version format '$VERSION'; must be like v1.2.3" >&2
        exit 1
        ;;
    esac
  else
    LATEST_URL="$(curl -fsLI -o /dev/null -w '%{url_effective}' "https://github.com/${REPO}/releases/latest" || true)"
    LATEST_URL="${LATEST_URL%/}"
    VERSION="${LATEST_URL##*/}"
    case "$VERSION" in
      v[0-9]*.[0-9]*.[0-9]*) ;;
      *)
        echo "error: could not determine the latest release; set VAULTCHRON_VERSION=vX.Y.Z (see the Releases page)" >&2
        exit 1
        ;;
    esac
  fi

  BASE_URL="${VAULTCHRON_BASE_URL:-https://github.com/${REPO}/releases/download/${VERSION}}"
  VER_NUM="${VERSION#v}"
  ARCHIVE="vaultchron_${VER_NUM}_${OS}_${ARCH}.tar.gz"
  CHECKSUMS="checksums.txt"

  TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/vaultchron-install.XXXXXX")"

  curl -fsSL "${BASE_URL}/${ARCHIVE}" -o "${TMP_DIR}/${ARCHIVE}"
  curl -fsSL "${BASE_URL}/${CHECKSUMS}" -o "${TMP_DIR}/${CHECKSUMS}"

  EXPECTED_HASH="$(awk -v f="$ARCHIVE" '$2 == f { print tolower($1) }' "$TMP_DIR/$CHECKSUMS")"
  if [ -z "$EXPECTED_HASH" ]; then
    echo "error: checksum for $ARCHIVE not found in $CHECKSUMS" >&2
    exit 1
  fi

  if [ "$SHACMD" = "sha256sum" ]; then
    ACTUAL_HASH="$(sha256sum "${TMP_DIR}/${ARCHIVE}" | awk '{print tolower($1)}')"
  else
    ACTUAL_HASH="$(shasum -a 256 "${TMP_DIR}/${ARCHIVE}" | awk '{print tolower($1)}')"
  fi

  if [ "$ACTUAL_HASH" != "$EXPECTED_HASH" ]; then
    echo "error: checksum mismatch for $ARCHIVE" >&2
    echo "  expected: $EXPECTED_HASH" >&2
    echo "  actual:   $ACTUAL_HASH" >&2
    exit 1
  fi

  tar -xzf "${TMP_DIR}/${ARCHIVE}" -C "$TMP_DIR"

  if [ ! -f "${TMP_DIR}/vaultchron" ] || [ ! -f "${TMP_DIR}/vaultchron_migrate" ]; then
    echo "error: expected binaries not found in archive" >&2
    exit 1
  fi

  INSTALL_DIR="${VAULTCHRON_INSTALL_DIR:-$HOME/.local/bin}"
  INSTALL_DIR="${INSTALL_DIR%/}"

  mkdir -p "$INSTALL_DIR"
  install -m 0755 "${TMP_DIR}/vaultchron" "${INSTALL_DIR}/vaultchron"
  install -m 0755 "${TMP_DIR}/vaultchron_migrate" "${INSTALL_DIR}/vaultchron_migrate"

  echo "Installed vaultchron and vaultchron_migrate to $INSTALL_DIR"
  "$INSTALL_DIR/vaultchron" -version
  "$INSTALL_DIR/vaultchron_migrate" -version

  case ":$PATH:" in
    *":$INSTALL_DIR:"*) ;;
    *)
      echo "Note: $INSTALL_DIR is not on your PATH."
      echo "Add the following line to your shell profile (e.g. ~/.bashrc or ~/.zshrc):"
      echo "  export PATH=\"${INSTALL_DIR}:\$PATH\""
      ;;
  esac
}

main "$@"
