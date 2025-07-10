#!/bin/sh
#
# This script downloads and installs the latest hemmelig client for your system.
# It attempts to install to $HOME/.local/bin, a common user-specific bin directory.
#
# Usage:
#   curl -sSL https://raw.githubusercontent.com/bjarneo/hemmelig/main/install.sh | sh
#   or
#   wget -qO- https://raw.githubusercontent.com/bjarneo/hemmelig/main/install.sh | sh

set -e # Exit immediately if a command exits with a non-zero status.

# --- Configuration ---
REPO="bjarneo/hemmelig"
INSTALL_DIR="$HOME/.local/bin"

# --- Helper Functions ---
echo_info() {
  # ANSI escape codes for blue color
  printf "\033[34m[INFO]\033[0m %s\n" "$1"
}

echo_error() {
  # ANSI escape codes for red color
  printf "\033[31m[ERROR]\033[0m %s\n" "$1" >&2
  exit 1
}

# --- System Detection ---
detect_os() {
  case "$(uname -s)" in
  Linux) OS='linux' ;;
  Darwin) OS='darwin' ;;
  *) echo_error "Unsupported operating system: $(uname -s). Only Linux and macOS are supported." ;;
  esac
}

detect_arch() {
  case "$(uname -m)" in
  x86_64 | amd64) ARCH='amd64' ;;
  i386 | i686) ARCH='386' ;;
  *) echo_error "Unsupported architecture: $(uname -m)." ;;
  esac
}

# --- Main Logic ---
main() {
  detect_os
  detect_arch

  # The 386 build is only for Linux in this project's CI
  if [ "$OS" = "darwin" ] && [ "$ARCH" = "386" ]; then
    echo_error "32-bit macOS is not supported by this installer."
  fi

  echo_info "Detected System: ${OS}-${ARCH}"

  # Fetch the latest release tag from the GitHub API
  echo_info "Fetching latest release information..."
  LATEST_TAG=$(curl -s "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
  if [ -z "$LATEST_TAG" ]; then
    echo_error "Could not automatically find the latest release tag. Please install manually."
  fi
  echo_info "Latest release is ${LATEST_TAG}"

  # Construct asset names and download URLs
  BINARY_NAME="hemmelig-${OS}-${ARCH}"
  CHECKSUMS_FILE="checksums.txt"
  BASE_URL="https://github.com/${REPO}/releases/download/${LATEST_TAG}"
  BINARY_URL="${BASE_URL}/${BINARY_NAME}"
  CHECKSUMS_URL="${BASE_URL}/${CHECKSUMS_FILE}"

  # Create a temporary directory for downloads
  TMP_DIR=$(mktemp -d)
  trap 'rm -rf "$TMP_DIR"' EXIT # Ensure cleanup on script exit

  echo_info "Downloading assets to a temporary directory..."
  curl -sSL -o "$TMP_DIR/$BINARY_NAME" "$BINARY_URL"
  curl -sSL -o "$TMP_DIR/$CHECKSUMS_FILE" "$CHECKSUMS_URL"

  echo_info "Verifying checksum..."
  (cd "$TMP_DIR" && grep "$BINARY_NAME" "$CHECKSUMS_FILE" | sha256sum -c -)
  if [ $? -ne 0 ]; then
    echo_error "Checksum verification failed! The downloaded file may be corrupt."
  fi
  echo_info "Checksum verified successfully."

  echo_info "Installing hemmelig to ${INSTALL_DIR}..."
  mkdir -p "$INSTALL_DIR"
  chmod +x "$TMP_DIR/$BINARY_NAME"
  mv "$TMP_DIR/$BINARY_NAME" "$INSTALL_DIR/hemmelig"

  echo_info "Installation complete!"
  echo_info "Please ensure '${INSTALL_DIR}' is in your shell's PATH."
  echo_info "You may need to add the following line to your ~/.bashrc, ~/.zshrc, or ~/.profile:"
  echo_info "  export PATH=\"$HOME/.local/bin:\$PATH\""
  echo_info "Then, restart your shell or run 'source ~/.bashrc' (or equivalent)."
}

# --- Run ---
main
