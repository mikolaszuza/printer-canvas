#!/bin/sh
# What `make release` does, for machines without make.
set -e
cd "$(dirname "$0")"
NAME=$(sed -n 's/^name:[[:space:]]*//p' module.yaml | head -1)
(cd ui && pnpm run build)
rm -rf release && cp -R ui/dist release
cp LICENSE README.md CHANGELOG.md release/
for platform in ${PLATFORMS:-$(sh scripts/platforms.sh)}; do
  goos=${platform%%_*}; goarch=${platform#*_}
  binary=${NAME}_$platform; [ "$goos" = windows ] && binary=$binary.exe
  echo "building $binary"
  CGO_ENABLED=0 GOOS=$goos GOARCH=$goarch go build -trimpath -ldflags='-s -w' -o "release/$binary" .
done
echo "$NAME is in release/"
