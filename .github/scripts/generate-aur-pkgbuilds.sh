#!/usr/bin/env bash
# Generates an AUR PKGBUILD and .SRCINFO for a released tag. Output only; nothing is published.
# Usage: generate-aur-pkgbuilds.sh <tag> <outdir>
set -euo pipefail
[ $# -eq 2 ] || { echo "Usage: $0 <tag> <outdir>" >&2; exit 1; }
version="$1"; out="$2"
# pkgver cannot contain '-', so v1.6.0-rc1 becomes 1.6.0_rc1; the tag and the
# directory GitHub extracts the tarball to (whatscli-1.6.0-rc1) keep the dash.
tagver="${version#v}"; pkgver="${tagver//-/_}"
repo="PalWorks/whatscli"
url="https://github.com/${repo}/archive/${version}.tar.gz"
tmp="$(mktemp)"; trap 'rm -f "$tmp"' EXIT
curl -fsSL "$url" -o "$tmp"
sha="$(sha256sum "$tmp" | awk '{print $1}')"
mkdir -p "$out/whatscli"

cat > "$out/whatscli/PKGBUILD" <<PKG
# Maintainer: PalWorks
pkgname=whatscli
pkgver=${pkgver}
pkgrel=1
pkgdesc='A command line interface for WhatsApp, based on go-whatsmeow and tview'
arch=('x86_64' 'aarch64')
license=('AGPL-3.0-only')
url='https://github.com/${repo}'
makedepends=('go' 'git' 'gcc')
_tagver=${tagver}
source=("\${pkgname}-\${pkgver}.tar.gz::https://github.com/${repo}/archive/v\${_tagver}.tar.gz")
sha256sums=('${sha}')

build() {
    cd "\${pkgname}-\${_tagver}"
    export CGO_ENABLED=1
    export CGO_CPPFLAGS="\${CPPFLAGS}"
    export CGO_CFLAGS="\${CFLAGS}"
    export CGO_CXXFLAGS="\${CXXFLAGS}"
    export CGO_LDFLAGS="\${LDFLAGS}"
    export GOFLAGS="-buildmode=pie -trimpath -mod=readonly -modcacherw"
    go build -o "\${pkgname}" -ldflags "-s -w -X main.VERSION=v\${_tagver}" .
}

package() {
    install -Dm755 "\${pkgname}-\${_tagver}/\${pkgname}" "\${pkgdir}/usr/bin/\${pkgname}"
}
PKG

cat > "$out/whatscli/.SRCINFO" <<SRC
pkgbase = whatscli
	pkgdesc = A command line interface for WhatsApp, based on go-whatsmeow and tview
	pkgver = ${pkgver}
	pkgrel = 1
	url = https://github.com/${repo}
	arch = x86_64
	arch = aarch64
	makedepends = go
	makedepends = git
	makedepends = gcc
	source = whatscli-${pkgver}.tar.gz::https://github.com/${repo}/archive/v${tagver}.tar.gz
	sha256sums = ${sha}

pkgname = whatscli
SRC
