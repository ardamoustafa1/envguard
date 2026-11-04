#!/bin/sh
# EnvGuard installer: downloads a release binary from GitHub Releases,
# verifies its SHA-256 checksum and installs it.
#
#   curl -fsSL https://raw.githubusercontent.com/ardamoustafa1/envguard/main/install.sh | sh
#   curl -fsSL https://raw.githubusercontent.com/ardamoustafa1/envguard/main/install.sh | sh -s -- 1.2.3
#
# Usage: install.sh [VERSION]      VERSION: 1.2.3, v1.2.3 or latest (default)
#
# Environment:
#   ENVGUARD_INSTALL_DIR   where to put the binary (default: /usr/local/bin as
#                          root, else ~/.local/bin)
#   ENVGUARD_RELEASES_URL  release base URL, for mirrors
#                          (default: https://github.com/ardamoustafa1/envguard/releases)
#   ENVGUARD_VERIFY_SIGNATURE=1  also verify the Sigstore signature of
#                          checksums.txt with cosign (must be installed)
#
# POSIX sh; needs curl or wget, tar (unzip on Windows), and sha256sum,
# shasum or openssl.

set -eu

REPO="ardamoustafa1/envguard"
RELEASES_URL="${ENVGUARD_RELEASES_URL:-https://github.com/${REPO}/releases}"

say() { printf 'envguard-install: %s\n' "$*" >&2; }
die() {
	say "error: $*"
	exit 1
}

have() { command -v "$1" >/dev/null 2>&1; }

# fetch URL FILE
fetch() {
	if have curl; then
		curl -fsSL --retry 3 -o "$2" "$1"
	elif have wget; then
		wget -q -O "$2" "$1"
	else
		die "curl or wget is required"
	fi
}

# latest_version prints the newest release version (without "v"), following
# the /releases/latest redirect.
latest_version() {
	if have curl; then
		url=$(curl -fsSLI -o /dev/null -w '%{url_effective}' "${RELEASES_URL}/latest") ||
			die "could not resolve the latest release"
	elif have wget; then
		url=$(wget -S --spider "${RELEASES_URL}/latest" 2>&1 | sed -n 's/^ *[Ll]ocation: *//p' | tail -n 1 | tr -d '\r')
	else
		die "curl or wget is required"
	fi
	tag=${url##*/}
	case "$tag" in
	v[0-9]*) printf '%s\n' "${tag#v}" ;;
	*) die "could not resolve the latest release (got '${tag}')" ;;
	esac
}

detect_os() {
	case "$(uname -s)" in
	Linux) echo linux ;;
	Darwin) echo darwin ;;
	MINGW* | MSYS* | CYGWIN* | Windows_NT) echo windows ;;
	*) die "unsupported operating system $(uname -s); see https://github.com/${REPO}/blob/main/docs/installation.md" ;;
	esac
}

detect_arch() {
	arch=$(uname -m)
	# An x86_64 shell under Rosetta on Apple silicon: prefer the native binary.
	if [ "$1" = darwin ] && [ "$arch" = x86_64 ] &&
		[ "$(sysctl -n hw.optional.arm64 2>/dev/null || echo 0)" = 1 ]; then
		arch=arm64
	fi
	case "$arch" in
	x86_64 | amd64) echo amd64 ;;
	aarch64 | arm64 | armv8*) echo arm64 ;;
	*) die "unsupported architecture ${arch}; prebuilt binaries exist for amd64 and arm64" ;;
	esac
}

sha256() {
	if have sha256sum; then
		sha256sum "$1" | cut -d ' ' -f 1
	elif have shasum; then
		shasum -a 256 "$1" | cut -d ' ' -f 1
	elif have openssl; then
		openssl dgst -sha256 "$1" | sed 's/^.*= *//'
	else
		die "sha256sum, shasum or openssl is required to verify the download"
	fi
}

default_install_dir() {
	if [ "$(id -u 2>/dev/null || echo 1)" = 0 ]; then
		echo /usr/local/bin
	else
		echo "${HOME}/.local/bin"
	fi
}

main() {
	version=${1:-latest}
	case "$version" in
	-h | --help)
		sed -n '2,20p' "$0" 2>/dev/null | sed 's/^# \{0,1\}//'
		exit 0
		;;
	latest) version=$(latest_version) ;;
	v[0-9]*) version=${version#v} ;;
	[0-9]*) ;;
	*) die "invalid version '${version}': use 1.2.3, v1.2.3 or latest" ;;
	esac

	os=$(detect_os)
	arch=$(detect_arch "$os")
	ext=tar.gz
	exe=envguard
	if [ "$os" = windows ]; then
		ext=zip
		exe=envguard.exe
	fi
	archive="envguard_${version}_${os}_${arch}.${ext}"
	base="${RELEASES_URL}/download/v${version}"

	tmp=$(mktemp -d 2>/dev/null || mktemp -d -t envguard)
	trap 'rm -rf "$tmp"' EXIT INT TERM

	say "downloading EnvGuard ${version} (${os}/${arch})"
	fetch "${base}/${archive}" "${tmp}/${archive}" || die "download failed: ${base}/${archive}"
	fetch "${base}/checksums.txt" "${tmp}/checksums.txt" || die "download failed: ${base}/checksums.txt"

	if [ "${ENVGUARD_VERIFY_SIGNATURE:-0}" = 1 ]; then
		have cosign || die "ENVGUARD_VERIFY_SIGNATURE=1 needs cosign"
		fetch "${base}/checksums.txt.sigstore.json" "${tmp}/checksums.txt.sigstore.json" ||
			die "download failed: checksums.txt.sigstore.json"
		cosign verify-blob \
			--bundle "${tmp}/checksums.txt.sigstore.json" \
			--certificate-identity-regexp "^https://github.com/${REPO}/\.github/workflows/release\.yml@refs/tags/v" \
			--certificate-oidc-issuer https://token.actions.githubusercontent.com \
			"${tmp}/checksums.txt" >/dev/null 2>&1 || die "the signature of checksums.txt is invalid"
		say "signature of checksums.txt verified"
	fi

	expected=$(awk -v f="$archive" '$2 == f || $2 == "*" f { print $1 }' "${tmp}/checksums.txt")
	[ -n "$expected" ] || die "${archive} is not listed in checksums.txt"
	actual=$(sha256 "${tmp}/${archive}")
	[ "$expected" = "$actual" ] || die "checksum mismatch for ${archive} (expected ${expected}, got ${actual})"
	say "checksum verified"

	mkdir -p "${tmp}/x"
	if [ "$ext" = zip ]; then
		have unzip || die "unzip is required"
		unzip -q "${tmp}/${archive}" -d "${tmp}/x"
	else
		tar -xzf "${tmp}/${archive}" -C "${tmp}/x"
	fi
	[ -f "${tmp}/x/${exe}" ] || die "${exe} not found in ${archive}"

	dir=${ENVGUARD_INSTALL_DIR:-$(default_install_dir)}
	mkdir -p "$dir" || die "cannot create ${dir}; set ENVGUARD_INSTALL_DIR or run with sudo"
	[ -w "$dir" ] || die "${dir} is not writable; set ENVGUARD_INSTALL_DIR or run with sudo"
	# Copy next to the target, then rename: atomic, even over a running binary.
	cp "${tmp}/x/${exe}" "${dir}/.${exe}.new"
	chmod 755 "${dir}/.${exe}.new"
	mv -f "${dir}/.${exe}.new" "${dir}/${exe}"

	say "installed ${dir}/${exe}"
	"${dir}/${exe}" version >&2 || true
	case ":${PATH}:" in
	*":${dir}:"*) ;;
	*) say "note: ${dir} is not on your PATH; add it, e.g. export PATH=\"${dir}:\$PATH\"" ;;
	esac
}

main "$@"
