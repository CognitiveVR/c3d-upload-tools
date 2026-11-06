#!/bin/bash
set -euo pipefail

OS="$1"
BINARY_PATH="$2"

# Only sign macOS binaries
if [ "$OS" != "darwin" ]; then
  echo "Skipping signing for $OS"
  exit 0
fi

# Skip signing if secrets are not available (e.g. snapshot CI builds),
# unless the caller requires signing (REQUIRE_SIGNING=1 on real releases)
if [ -z "${APPLE_TEAM_ID:-}" ]; then
  if [ "${REQUIRE_SIGNING:-}" = "1" ]; then
    echo "Error: APPLE_TEAM_ID is not set but REQUIRE_SIGNING=1; refusing to ship an unsigned binary" >&2
    exit 1
  fi
  echo "Skipping signing: APPLE_TEAM_ID not set"
  exit 0
fi

SIGN_IDENTITY="Developer ID Application: CognitiveVR Inc. ($APPLE_TEAM_ID)"

echo "Signing $BINARY_PATH..."
codesign --force --options runtime \
  --sign "$SIGN_IDENTITY" \
  --timestamp \
  "$BINARY_PATH"

echo "Verifying signature..."
codesign --verify --verbose "$BINARY_PATH"

echo "Submitting for notarization..."
NOTARIZE_ZIP="${BINARY_PATH}.notarize.zip"
trap 'rm -f "$NOTARIZE_ZIP"' EXIT
zip -j "$NOTARIZE_ZIP" "$BINARY_PATH"

# notarytool can exit 0 for a rejected submission, so the status field decides
if ! RESULT=$(xcrun notarytool submit "$NOTARIZE_ZIP" \
  --apple-id "$APPLE_ID" \
  --password "$APPLE_ID_PASSWORD" \
  --team-id "$APPLE_TEAM_ID" \
  --wait \
  --output-format json); then
  echo "Error: notarytool submit failed: $RESULT" >&2
  exit 1
fi

SUBMISSION_ID=$(printf '%s' "$RESULT" | plutil -extract id raw -o - - 2>/dev/null || echo "unknown")
STATUS=$(printf '%s' "$RESULT" | plutil -extract status raw -o - - 2>/dev/null || echo "unknown")
echo "Notarization submission $SUBMISSION_ID: $STATUS"

if [ "$STATUS" != "Accepted" ]; then
  echo "Error: notarization of $BINARY_PATH was not accepted (status: $STATUS)" >&2
  echo "Next step: xcrun notarytool log $SUBMISSION_ID --apple-id \"\$APPLE_ID\" --password \"\$APPLE_ID_PASSWORD\" --team-id \"\$APPLE_TEAM_ID\"" >&2
  exit 1
fi

echo "Signing and notarization complete for $BINARY_PATH"
