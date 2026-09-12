#!/bin/bash
# Installs (or updates) the tools binary from the latest GitHub Release,
# mirroring the `curl -fsSL https://claude.ai/install.sh | bash` install
# pattern.
#
# Usage: curl -fsSL <raw-url>/install.sh | bash
set -euo pipefail

TOOLS_OWNER="bruaguspons"
TOOLS_REPO="tools"
TOOLS_BIN_DIR="${HOME}/.local/bin"
TOOLS_BIN_NAME="tools"

# validate_asset_filename rejects any asset filename that does not match
# the exact goreleaser-produced pattern for a supported platform, before
# any extraction happens. This is the threat-matrix guard against
# extracting an unexpected/malformed asset (e.g. path traversal, wrong
# OS/arch, wrong extension).
#
# Pattern: tools_<version>_linux_<amd64|arm64>.tar.gz
validate_asset_filename() {
  local filename="$1"
  [[ -n "${filename}" ]] || return 1
  [[ "${filename}" =~ ^tools_[0-9]+\.[0-9]+\.[0-9]+_linux_(amd64|arm64)\.tar\.gz$ ]]
}

detect_arch() {
  local machine
  machine="$(uname -m)"
  case "${machine}" in
    x86_64 | amd64) echo "amd64" ;;
    aarch64 | arm64) echo "arm64" ;;
    *)
      echo "tools: unsupported architecture '${machine}'" >&2
      return 1
      ;;
  esac
}

detect_os() {
  local kernel
  kernel="$(uname -s)"
  case "${kernel}" in
    Linux) echo "linux" ;;
    *)
      echo "tools: unsupported OS '${kernel}' (only linux is supported)" >&2
      return 1
      ;;
  esac
}

install_main() {
  local os arch api_url latest_json tag asset_name asset_url archive_path

  os="$(detect_os)"
  arch="$(detect_arch)"

  api_url="https://api.github.com/repos/${TOOLS_OWNER}/${TOOLS_REPO}/releases/latest"
  latest_json="$(curl -fsSL "${api_url}")"

  tag="$(printf '%s' "${latest_json}" | grep -o '"tag_name"[[:space:]]*:[[:space:]]*"[^"]*"' | head -1 | sed -E 's/.*"([^"]+)"$/\1/')"
  if [[ -z "${tag}" ]]; then
    echo "tools: could not determine latest release tag" >&2
    return 1
  fi

  local version="${tag#v}"
  asset_name="tools_${version}_${os}_${arch}.tar.gz"

  if ! validate_asset_filename "${asset_name}"; then
    echo "tools: refusing to install unexpected/malformed asset name '${asset_name}'" >&2
    return 1
  fi

  asset_url="https://github.com/${TOOLS_OWNER}/${TOOLS_REPO}/releases/download/${tag}/${asset_name}"

  tmp_dir="$(mktemp -d)"
  trap 'rm -rf "${tmp_dir}"' EXIT

  archive_path="${tmp_dir}/${asset_name}"
  curl -fsSL -o "${archive_path}" "${asset_url}"

  mkdir -p "${TOOLS_BIN_DIR}"
  tar -xzf "${archive_path}" -C "${tmp_dir}"

  if [[ ! -f "${tmp_dir}/${TOOLS_BIN_NAME}" ]]; then
    echo "tools: extracted archive is missing the expected binary" >&2
    return 1
  fi

  # Stage the downloaded binary inside the destination bin dir, then
  # rename it into place. Staging in the SAME directory as the final
  # target guarantees the rename is a true same-filesystem atomic swap:
  # readers/writers of the old binary either see the fully-old or
  # fully-new file, never a half-written one, and there is no ETXTBSY
  # against a running process the way an in-place truncating install
  # would risk.
  local staged="${TOOLS_BIN_DIR}/.tools.tmp.$$"
  cp "${tmp_dir}/${TOOLS_BIN_NAME}" "${staged}"
  chmod 0755 "${staged}"
  mv "${staged}" "${TOOLS_BIN_DIR}/${TOOLS_BIN_NAME}"

  echo "tools: installed ${tag} to ${TOOLS_BIN_DIR}/${TOOLS_BIN_NAME}"
}

# Allow this script to be `source`d for unit testing its functions
# (e.g. validate_asset_filename) without running the network install flow.
if [[ "${TOOLS_SOURCE_ONLY:-}" != "1" ]]; then
  install_main "$@"
fi
