#!/bin/bash
set -euo pipefail

OS="$1"
BINARY_PATH="$2"

# Only sign macOS binaries
if [ "$OS" != "darwin" ]; then
  echo "Skipping signing for $OS"
  exit 0
fi

# Skip signing if secrets are not available (e.g. snapshot CI builds)
if [ -z "${APPLE_TEAM_ID:-}" ]; then
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
zip -j "$NOTARIZE_ZIP" "$BINARY_PATH"

xcrun notarytool submit "$NOTARIZE_ZIP" \
  --apple-id "$APPLE_ID" \
  --password "$APPLE_ID_PASSWORD" \
  --team-id "$APPLE_TEAM_ID" \
  --wait

rm "$NOTARIZE_ZIP"
echo "Signing and notarization complete for $BINARY_PATH"
