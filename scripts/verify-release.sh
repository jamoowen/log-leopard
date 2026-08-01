#!/bin/sh
set -eu

tag=${1:-}
case "$tag" in
v[0-9]*.[0-9]*.[0-9]*) ;;
*)
  echo "usage: $0 vX.Y.Z[-prerelease]" >&2
  exit 2
  ;;
esac

for command in gh grep mktemp tar; do
  if ! command -v "$command" >/dev/null 2>&1; then
    echo "required command not found: $command" >&2
    exit 1
  fi
done

case "$(uname -s)" in
Darwin) os=darwin ;;
Linux) os=linux ;;
*)
  echo "release verification currently supports macOS and Linux" >&2
  exit 1
  ;;
esac

case "$(uname -m)" in
x86_64 | amd64) arch=amd64 ;;
arm64 | aarch64) arch=arm64 ;;
*)
  echo "unsupported architecture: $(uname -m)" >&2
  exit 1
  ;;
esac

version=${tag#v}
archive="log-leopard_${version}_${os}_${arch}.tar.gz"
tmp=$(mktemp -d "${TMPDIR:-/tmp}/log-leopard-release.XXXXXX")
pid=

cleanup() {
  if [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; then
    kill "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
  fi
  rm -rf "$tmp"
}
trap cleanup EXIT HUP INT TERM

assets=$(gh release view "$tag" --json assets --jq '.assets[].name')
for expected in \
  "log-leopard_${version}_darwin_amd64.tar.gz" \
  "log-leopard_${version}_darwin_arm64.tar.gz" \
  "log-leopard_${version}_linux_amd64.tar.gz" \
  "log-leopard_${version}_linux_arm64.tar.gz" \
  "log-leopard_${version}_windows_amd64.zip" \
  "log-leopard_${version}_windows_arm64.zip" \
  SHA256SUMS; do
  if ! printf '%s\n' "$assets" | grep -Fx "$expected" >/dev/null; then
    echo "release asset missing: $expected" >&2
    exit 1
  fi
done

gh release download "$tag" --pattern "$archive" --pattern SHA256SUMS --dir "$tmp"

(
  cd "$tmp"
  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 -c SHA256SUMS --ignore-missing
  elif command -v sha256sum >/dev/null 2>&1; then
    sha256sum -c SHA256SUMS --ignore-missing
  else
    echo "required command not found: shasum or sha256sum" >&2
    exit 1
  fi
)

mkdir "$tmp/unpacked"
tar -xzf "$tmp/$archive" -C "$tmp/unpacked"
binary="$tmp/unpacked/log-leopard"
actual_version=$("$binary" -version)
case "$actual_version" in
*" $version ("*) ;;
*)
  echo "unexpected binary version: $actual_version" >&2
  exit 1
  ;;
esac

"$binary" -fake -config "$tmp/connections.json" >"$tmp/server.log" 2>&1 &
pid=$!
sleep 2
if ! kill -0 "$pid" 2>/dev/null; then
  echo "packaged binary exited during startup; output withheld because it contains a pairing token" >&2
  exit 1
fi
kill "$pid"
wait "$pid"
pid=

echo "Verified $tag: complete asset set, checksum, version, and ${os}/${arch} startup"
