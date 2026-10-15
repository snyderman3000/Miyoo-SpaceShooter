#!/bin/sh
# Builds the OnionOS package: dist/SpaceShooter-v<version>-OnionOS.zip
#   needs Go 1.24+ and zig (for the little C sound helper; `pip install ziglang` works)
set -e
cd "$(dirname "$0")"
VERSION=$(grep -o 'const version = "[^"]*"' main.go | cut -d'"' -f2)
if command -v zig >/dev/null 2>&1; then ZIG="zig"; else ZIG="python3 -m ziglang"; fi

go mod tidy
go vet ./...
go test ./...
CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build -trimpath -ldflags="-s -w" -o package/SpaceShooter/spaceshooter .
# glibc 2.23 target: the helper only needs symbols every Miyoo firmware has
$ZIG cc -target arm-linux-gnueabihf.2.23 -mcpu=cortex_a7 -O2 -s sfx/sfxd.c -o package/SpaceShooter/sfxd

rm -rf dist && mkdir -p dist/stage/App
cp -r package/SpaceShooter dist/stage/App/
rm -rf dist/stage/App/SpaceShooter/data
(cd dist/stage && zip -qr "../SpaceShooter-v$VERSION-OnionOS.zip" App)
rm -rf dist/stage
echo "Built dist/SpaceShooter-v$VERSION-OnionOS.zip"
