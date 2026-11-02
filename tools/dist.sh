#!/bin/sh
# Builds release archives into dist/: one ollametry_<version>_<os>_<arch>.tar.gz
# per platform with the binary, LICENSE, THIRD_PARTY_NOTICES.md and README.md
# (macOS archives add the launchd agent), plus SHA256SUMS. Run it through
# `make dist`, inside the pinned golang image, so the binaries match the
# container image's toolchain and flags.
#
# Archives are reproducible: sorted names, fixed mtimes (SOURCE_DATE_EPOCH, the
# last commit time), numeric root ownership and gzip without a timestamp, so
# rebuilding a tag yields the same checksums.
set -eu

: "${VERSION:?VERSION is required, e.g. v0.1.0}"
: "${PLATFORMS:?PLATFORMS is required, e.g. darwin/arm64 linux/amd64}"
: "${SOURCE_DATE_EPOCH:?SOURCE_DATE_EPOCH is required (last commit time)}"

name_version=${VERSION#v}
out=dist
staging=$(mktemp -d)
trap 'rm -rf "$staging"' EXIT

rm -rf "$out"
mkdir -p "$out"

for platform in $PLATFORMS; do
	os=${platform%/*}
	arch=${platform#*/}
	dir="ollametry_${name_version}_${os}_${arch}"
	mkdir -p "$staging/$dir"

	echo "==> $os/$arch"
	CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath -buildvcs=false \
		-ldflags="-s -w -X main.version=$VERSION" \
		-o "$staging/$dir/ollametry" ./cmd/ollametry
	cp LICENSE THIRD_PARTY_NOTICES.md README.md "$staging/$dir/"
	if [ "$os" = darwin ]; then
		cp deploy/launchd/io.github.a1exus.ollametry.plist "$staging/$dir/"
	fi

	tar --sort=name --mtime="@$SOURCE_DATE_EPOCH" --owner=0 --group=0 --numeric-owner \
		-C "$staging" -cf - "$dir" | gzip -9 -n > "$out/$dir.tar.gz"
done

(cd "$out" && sha256sum -- *.tar.gz > SHA256SUMS)
cat "$out/SHA256SUMS"
