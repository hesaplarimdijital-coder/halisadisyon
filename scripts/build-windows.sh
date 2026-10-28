#!/usr/bin/env bash
# Windows kurulum dosyasını Linux'ta üretir.
# Gerekenler: Go 1.24+, x86_64-w64-mingw32-windres (binutils-mingw-w64), makensis (nsis)
set -euo pipefail
cd "$(dirname "$0")/.."

VERSION="${VERSION:-2.3.1}"
DIST="dist"
STAGE="$DIST/stage"
rm -rf "$STAGE"
mkdir -p "$STAGE"

echo "== Windows kaynakları (simge, manifest, sürüm)"
# Sürüm numarası app.rc'ye buradan yazılır (1.2.3 -> 1,2,3,0 ve "1.2.3").
IFS=. read -r V1 V2 V3 _ <<<"$VERSION"
sed -e "s/1,0,0,0/${V1:-0},${V2:-0},${V3:-0},0/g" -e "s/\"1\.0\.0\"/\"$VERSION\"/g" \
  build/windows/app.rc > build/windows/app.gen.rc
( cd build/windows && x86_64-w64-mingw32-windres --preprocessor=cpp -c 65001 -i app.gen.rc -O coff \
    -o ../../cmd/halisadisyon/rsrc_windows_amd64.syso )
rm -f build/windows/app.gen.rc

echo "== Testler"
go test ./...

echo "== HalisAdisyon.exe"
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath \
  -ldflags "-H windowsgui -s -w -X main.version=$VERSION" \
  -o "$STAGE/HalisAdisyon.exe" ./cmd/halisadisyon

# Metin dosyaları: UTF-8 (BOM'lu) ve Windows satır sonu
for f in Kullanim_Rehberi.txt Lisanslar.txt; do
  { printf '\xEF\xBB\xBF'; sed 's/\r$//; s/$/\r/' "build/windows/$f"; } > "$STAGE/$f"
done
cp build/windows/app.ico "$STAGE/app.ico"

echo "== Kurulum dosyası"
OUT="$(pwd)/$DIST/HalisAdisyon_Kurulum_$VERSION.exe"
{ printf '\xEF\xBB\xBF'; cat build/windows/installer.nsi; } > "$STAGE/installer.nsi"
makensis -V2 -DSTAGE="$(pwd)/$STAGE" -DVERSION="$VERSION" -DOUTFILE="$OUT" "$STAGE/installer.nsi"
ls -la "$OUT"
