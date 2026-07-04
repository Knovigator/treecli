#!/usr/bin/env sh
set -eu

repo="${TREECLI_REPO:-${TREECTL_REPO:-Knovigator/treecli}}"
install_dir="${TREECLI_INSTALL_DIR:-${TREECTL_INSTALL_DIR:-$HOME/.local/bin}}"
version="${TREECLI_VERSION:-${TREECTL_VERSION:-latest}}"
install_legacy="${TREECLI_INSTALL_LEGACY:-${TREECTL_INSTALL_LEGACY:-1}}"

need() {
    if ! command -v "$1" >/dev/null 2>&1; then
        echo "treecli installer requires $1" >&2
        exit 1
    fi
}

need curl
need tar

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$os" in
    darwin|linux) ;;
    *)
        echo "unsupported operating system: $os" >&2
        exit 1
        ;;
esac

arch="$(uname -m)"
case "$arch" in
    x86_64|amd64) arch="amd64" ;;
    arm64|aarch64) arch="arm64" ;;
    *)
        echo "unsupported architecture: $arch" >&2
        exit 1
        ;;
esac

if [ "$version" = "latest" ]; then
    tag="$(
        curl -fsSL "https://api.github.com/repos/${repo}/releases?per_page=100" |
            sed -n 's/.*"tag_name": "\(v[^"]*\)".*/\1/p' |
            head -n 1
    )"
    if [ -z "$tag" ]; then
        echo "could not find a treecli release for ${repo}" >&2
        exit 1
    fi
else
    case "$version" in
        v*) tag="$version" ;;
        *) tag="v$version" ;;
    esac
fi

asset="treecli_${tag}_${os}_${arch}.tar.gz"
legacy_asset="treectl_${tag}_${os}_${arch}.tar.gz"
base_url="https://github.com/${repo}/releases/download/${tag}"

tmpdir="$(mktemp -d)"
cleanup() {
    rm -rf "$tmpdir"
}
trap cleanup EXIT INT TERM

echo "Downloading ${asset} from ${repo} ${tag}"
if curl -fL "${base_url}/${asset}" -o "${tmpdir}/${asset}"; then
    :
else
    echo "Could not download ${asset}; trying legacy ${legacy_asset}"
    asset="$legacy_asset"
    curl -fL "${base_url}/${asset}" -o "${tmpdir}/${asset}"
fi
curl -fsSL "${base_url}/checksums.txt" -o "${tmpdir}/checksums.txt"

expected="$(grep " ${asset}$" "${tmpdir}/checksums.txt" | awk '{print $1}')"
if [ -z "$expected" ]; then
    echo "could not find checksum for ${asset}" >&2
    exit 1
fi

if command -v shasum >/dev/null 2>&1; then
    actual="$(shasum -a 256 "${tmpdir}/${asset}" | awk '{print $1}')"
elif command -v sha256sum >/dev/null 2>&1; then
    actual="$(sha256sum "${tmpdir}/${asset}" | awk '{print $1}')"
else
    echo "neither shasum nor sha256sum is available for checksum verification" >&2
    exit 1
fi

if [ "$expected" != "$actual" ]; then
    echo "checksum mismatch for ${asset}" >&2
    exit 1
fi

tar -C "$tmpdir" -xzf "${tmpdir}/${asset}"
mkdir -p "$install_dir"

binary="${tmpdir}/treecli"
if [ ! -f "$binary" ] && [ -f "${tmpdir}/treectl" ]; then
    binary="${tmpdir}/treectl"
fi
if [ ! -f "$binary" ]; then
    echo "archive did not contain treecli" >&2
    exit 1
fi

if command -v install >/dev/null 2>&1; then
    install -m 0755 "$binary" "${install_dir}/treecli"
    if [ "$install_legacy" != "0" ]; then
        install -m 0755 "$binary" "${install_dir}/treectl"
    fi
else
    cp "$binary" "${install_dir}/treecli"
    chmod 0755 "${install_dir}/treecli"
    if [ "$install_legacy" != "0" ]; then
        cp "$binary" "${install_dir}/treectl"
        chmod 0755 "${install_dir}/treectl"
    fi
fi

echo "Installed treecli to ${install_dir}/treecli"
if [ "$install_legacy" != "0" ]; then
    echo "Installed compatibility command to ${install_dir}/treectl"
fi
case ":$PATH:" in
    *":${install_dir}:"*) ;;
    *)
        echo "Add ${install_dir} to PATH to run treecli from any directory."
        ;;
esac
