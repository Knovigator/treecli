#!/usr/bin/env sh
set -eu

version="${1:-${TREECLI_VERSION:-${TREECTL_VERSION:-}}}"
if [ -z "$version" ]; then
    echo "usage: scripts/build-release.sh <vVERSION>" >&2
    exit 2
fi
case "$version" in
    v*) ;;
    *)
        echo "release version must start with v: $version" >&2
        exit 2
        ;;
esac

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo_root="${TREECLI_REPO_ROOT:-$(dirname "$script_dir")}"
dist_dir="${TREECLI_DIST_DIR:-${repo_root}/dist}"

if [ -f "${repo_root}/treecli.go" ]; then
    primary_name="treecli"
    legacy_name="treectl"
else
    primary_name="treectl"
    legacy_name=""
fi

mkdir -p "$dist_dir"
if find "$dist_dir" -mindepth 1 -maxdepth 1 -print -quit | grep -q .; then
    echo "release output directory must be empty: $dist_dir" >&2
    exit 2
fi

module_path=$(cd "$repo_root" && go list -m)
version_ldflag=""
if grep -R -q 'CurrentVersion' "${repo_root}/cmd" --include='*.go'; then
    version_ldflag="-X ${module_path}/cmd.CurrentVersion=${version}"
fi

build_target() {
    target_os="$1"
    target_arch="$2"
    binary_ext=""
    archive_ext="tar.gz"
    if [ "$target_os" = "windows" ]; then
        binary_ext=".exe"
        archive_ext="zip"
    fi

    package_name="${primary_name}_${version}_${target_os}_${target_arch}"
    package_dir="${dist_dir}/${package_name}"
    mkdir -p "$package_dir"

    (
        cd "$repo_root"
        CGO_ENABLED=0 GOOS="$target_os" GOARCH="$target_arch" go build \
            -trimpath \
            -ldflags "-s -w ${version_ldflag}" \
            -o "${package_dir}/${primary_name}${binary_ext}" \
            .
    )
    if [ -n "$legacy_name" ]; then
        cp "${package_dir}/${primary_name}${binary_ext}" "${package_dir}/${legacy_name}${binary_ext}"
    fi

    if [ "$archive_ext" = "zip" ]; then
        (cd "$package_dir" && zip -q -r "${dist_dir}/${package_name}.zip" .)
    else
        tar -C "$package_dir" -czf "${dist_dir}/${package_name}.tar.gz" .
    fi
    find "$package_dir" -depth -delete

    if [ -n "$legacy_name" ]; then
        legacy_package="${legacy_name}_${version}_${target_os}_${target_arch}"
        cp "${dist_dir}/${package_name}.${archive_ext}" "${dist_dir}/${legacy_package}.${archive_ext}"
    fi
}

build_target darwin amd64
build_target darwin arm64
build_target linux amd64
build_target linux arm64
build_target windows amd64

(
    cd "$dist_dir"
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum *.tar.gz *.zip > checksums.txt
    elif command -v shasum >/dev/null 2>&1; then
        shasum -a 256 *.tar.gz *.zip > checksums.txt
    else
        echo "sha256sum or shasum is required" >&2
        exit 1
    fi
)

echo "Built release artifacts in $dist_dir"
