#!/usr/bin/env sh
set -eu

version="${1:-${TREECLI_VERSION:-${TREECTL_VERSION:-}}}"
if [ -z "$version" ]; then
    echo "usage: scripts/verify-release.sh <vVERSION> [dist-dir]" >&2
    exit 2
fi

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo_root="${TREECLI_REPO_ROOT:-$(dirname "$script_dir")}"
dist_dir="${2:-${TREECLI_DIST_DIR:-${repo_root}/dist}}"

if [ -f "${repo_root}/treecli.go" ]; then
    primary_name="treecli"
    legacy_name="treectl"
else
    primary_name="treectl"
    legacy_name=""
fi

for target in darwin_amd64 darwin_arm64 linux_amd64 linux_arm64 windows_amd64; do
    archive_ext="tar.gz"
    case "$target" in
        windows_*) archive_ext="zip" ;;
    esac
    archive="${dist_dir}/${primary_name}_${version}_${target}.${archive_ext}"
    if [ ! -f "$archive" ]; then
        echo "missing release archive: $archive" >&2
        exit 1
    fi

    if [ -n "$legacy_name" ]; then
        legacy_archive="${dist_dir}/${legacy_name}_${version}_${target}.${archive_ext}"
        if [ ! -f "$legacy_archive" ]; then
            echo "missing legacy release archive: $legacy_archive" >&2
            exit 1
        fi
        if ! cmp -s "$archive" "$legacy_archive"; then
            echo "legacy archive does not match the primary archive: $legacy_archive" >&2
            exit 1
        fi
    fi

    if [ "$archive_ext" = "zip" ]; then
        unzip -Z1 "$archive" | grep -Eq "(^|/)${primary_name}\.exe$"
        if [ -n "$legacy_name" ]; then
            unzip -Z1 "$archive" | grep -Eq "(^|/)${legacy_name}\.exe$"
        fi
    else
        tar -tzf "$archive" | grep -Eq "(^|/)${primary_name}$"
        if [ -n "$legacy_name" ]; then
            tar -tzf "$archive" | grep -Eq "(^|/)${legacy_name}$"
        fi
    fi
done

(
    cd "$dist_dir"
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum -c checksums.txt
    elif command -v shasum >/dev/null 2>&1; then
        shasum -a 256 -c checksums.txt
    else
        echo "sha256sum or shasum is required" >&2
        exit 1
    fi
)

native_os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$native_os" in
    darwin|linux) ;;
    *)
        echo "Archive checks passed; native execution is not supported on $native_os"
        exit 0
        ;;
esac
native_arch=$(uname -m)
case "$native_arch" in
    x86_64|amd64) native_arch="amd64" ;;
    arm64|aarch64) native_arch="arm64" ;;
    *)
        echo "Archive checks passed; native execution is not supported on $native_arch"
        exit 0
        ;;
esac

verify_tmp=$(mktemp -d)
trap 'find "$verify_tmp" -depth -delete' EXIT INT TERM
native_archive="${dist_dir}/${primary_name}_${version}_${native_os}_${native_arch}.tar.gz"
tar -C "$verify_tmp" -xzf "$native_archive"
native_binary="${verify_tmp}/${primary_name}"
if [ ! -x "$native_binary" ]; then
    echo "native archive did not contain an executable ${primary_name}" >&2
    exit 1
fi
"$native_binary" --help >/dev/null

if grep -R -q 'CurrentVersion' "${repo_root}/cmd" --include='*.go'; then
    version_output=$("$native_binary" --version)
    echo "$version_output" | grep -F "$version" >/dev/null
fi

echo "Verified release artifacts in $dist_dir"
