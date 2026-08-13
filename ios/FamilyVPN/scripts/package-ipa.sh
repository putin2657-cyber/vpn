#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
APP_SRC="$ROOT/build/DerivedData/Build/Products/Release-iphoneos/FamilyVPN.app"
DIST="$ROOT/dist"
SIGN="$ROOT/signing"
IDENTITY="Apple Distribution: Lee Jone (9363A386MV)"

cd "$ROOT"
xcodegen generate
xcodebuild -project FamilyVPN.xcodeproj -scheme FamilyVPN \
  -destination 'generic/platform=iOS' \
  -configuration Release \
  -derivedDataPath build/DerivedData \
  build \
  CODE_SIGNING_ALLOWED=NO \
  CODE_SIGNING_REQUIRED=NO

rm -rf "$DIST/Payload"
mkdir -p "$DIST/Payload"
cp -R "$APP_SRC" "$DIST/Payload/FamilyVPN.app"
cp "$SIGN/profile.mobileprovision" "$DIST/Payload/FamilyVPN.app/embedded.mobileprovision"
cp "$SIGN/profile.mobileprovision" "$DIST/Payload/FamilyVPN.app/PlugIns/Tunnel.appex/embedded.mobileprovision"

codesign --force --sign "$IDENTITY" --entitlements "$ROOT/Tunnel/Tunnel.entitlements" --timestamp=none \
  "$DIST/Payload/FamilyVPN.app/PlugIns/Tunnel.appex"
codesign --force --sign "$IDENTITY" --entitlements "$ROOT/FamilyVPN/FamilyVPN.entitlements" --timestamp=none \
  "$DIST/Payload/FamilyVPN.app"

(
  cd "$DIST"
  rm -f FamilyVPN.ipa
  zip -qr FamilyVPN.ipa Payload
)
cp "$DIST/FamilyVPN.ipa" "$HOME/Downloads/FamilyVPN.ipa"
echo "IPA: $DIST/FamilyVPN.ipa"
