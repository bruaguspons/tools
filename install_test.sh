#!/bin/bash
# Plain-bash assertion script (RED test, threat-matrix item) for install.sh's
# asset-filename validation. Run with: bash install_test.sh
#
# It sources install.sh with TOOLS_SOURCE_ONLY=1 set, which must
# make the script define its functions (including validate_asset_filename)
# and return without executing the download/install flow. This lets us
# unit-test the filename guard without any network access.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
INSTALL_SH="${SCRIPT_DIR}/install.sh"
REPO_ROOT="${SCRIPT_DIR}"

if [[ ! -f "${INSTALL_SH}" ]]; then
  echo "FAIL: ${INSTALL_SH} does not exist yet" >&2
  exit 1
fi

# shellcheck source=/dev/null
TOOLS_SOURCE_ONLY=1 source "${INSTALL_SH}"

fail=0

assert_rejects() {
  local filename="$1"
  if validate_asset_filename "${filename}"; then
    echo "FAIL: expected rejection of malformed asset filename '${filename}'" >&2
    fail=1
  else
    echo "PASS: rejected malformed asset filename '${filename}'"
  fi
}

assert_accepts() {
  local filename="$1"
  if ! validate_asset_filename "${filename}"; then
    echo "FAIL: expected acceptance of well-formed asset filename '${filename}'" >&2
    fail=1
  else
    echo "PASS: accepted well-formed asset filename '${filename}'"
  fi
}

# Malformed / unexpected asset names must be rejected before extraction.
assert_rejects "evil.sh"
assert_rejects "tools_1.0.0_linux_amd64.zip"
assert_rejects "../../etc/passwd"
assert_rejects "tools_1.0.0_windows_amd64.tar.gz"
assert_rejects ""

# Well-formed release asset names for supported platforms must be accepted.
assert_accepts "tools_1.0.0_linux_amd64.tar.gz"
assert_accepts "tools_1.0.0_linux_arm64.tar.gz"

# The binary swap must be a stage-then-move (atomic within the destination
# bin dir), never a truncate-in-place install: an interrupted in-place
# write/truncate can leave a half-written binary (or fail with ETXTBSY
# against a running process), while an interrupted `mv` within the same
# directory fails as "old still present, new absent" but never
# "half-written binary at the final path".
assert_no_pattern_for_swap() {
  local pattern="$1"
  if grep -Eq "${pattern}" "${INSTALL_SH}"; then
    echo "FAIL: install.sh still contains the disallowed swap pattern '${pattern}'" >&2
    fail=1
  else
    echo "PASS: install.sh does not contain the disallowed swap pattern '${pattern}'"
  fi
}

assert_has_pattern() {
  local pattern="$1"
  if grep -Eq "${pattern}" "${INSTALL_SH}"; then
    echo "PASS: install.sh contains the required pattern '${pattern}'"
  else
    echo "FAIL: install.sh is missing the required pattern '${pattern}'" >&2
    fail=1
  fi
}

assert_no_pattern_for_swap 'install[[:space:]]+-m[[:space:]]+0755'
assert_has_pattern 'mv.*TOOLS_BIN_DIR'

# Acceptance gate (spec: no residue of the old project brand name): the old
# brand string must not remain anywhere in the repo tree. Built from two
# halves at runtime so this very check never itself becomes a match.
assert_no_old_brand_residue() {
  local half_a="dev"
  local half_b="sync"
  local old_brand="${half_a}${half_b}"
  local matches
  matches="$(grep -ril --exclude-dir=.git "${old_brand}" "${REPO_ROOT}" 2>/dev/null || true)"
  if [[ -n "${matches}" ]]; then
    echo "FAIL: found residual old-brand references:" >&2
    echo "${matches}" >&2
    fail=1
  else
    echo "PASS: no residual old-brand references found in the repo tree"
  fi
}

assert_no_old_brand_residue

if [[ "${fail}" -ne 0 ]]; then
  echo "install_test.sh: FAILURES DETECTED" >&2
  exit 1
fi

echo "install_test.sh: all assertions passed"
