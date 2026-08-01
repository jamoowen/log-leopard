#!/bin/sh

set -eu

root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
cd "$root"
tmp=$(mktemp -d "${TMPDIR:-/tmp}/log-leopard-dev.XXXXXX")
backend_pid=""
web_pid=""
browser=${LOG_LEOPARD_BROWSER:-}

cleanup() {
  trap - EXIT INT TERM HUP
  [ -z "$backend_pid" ] || kill "$backend_pid" 2>/dev/null || true
  [ -z "$web_pid" ] || kill "$web_pid" 2>/dev/null || true
  [ -z "$backend_pid" ] || wait "$backend_pid" 2>/dev/null || true
  [ -z "$web_pid" ] || wait "$web_pid" 2>/dev/null || true
  rm -rf "$tmp"
}

trap cleanup EXIT
trap 'exit 130' INT TERM HUP

check_port() {
  if ! node -e '
    const net = require("node:net");
    const server = net.createServer();
    server.once("error", () => process.exit(1));
    server.listen(Number(process.argv[1]), "127.0.0.1", () => server.close());
  ' "$1"; then
    printf '%s\n' "Port $1 is already in use. Stop the earlier LogLeopard development process and retry." >&2
    exit 1
  fi
}

check_port 5173
check_port 8787

if [ -n "$browser" ]; then
  case "$browser" in
    brave|chrome|firefox|safari) ;;
    *)
      printf '%s\n' "Unsupported browser '$browser'. Use brave, chrome, firefox, or safari." >&2
      exit 1
      ;;
  esac
  set -- -open -browser "$browser" "$@"
fi

go build -o "$tmp/log-leopard" ./cmd/log-leopard

node "$root/web/node_modules/vite/bin/vite.js" \
  --config "$root/vite.dev.config.mjs" \
  --host 127.0.0.1 \
  --port 5173 \
  --strictPort &
web_pid=$!

attempt=0
until node -e 'fetch("http://127.0.0.1:5173/").then(response => process.exit(response.ok ? 0 : 1)).catch(() => process.exit(1))'; do
  if ! kill -0 "$web_pid" 2>/dev/null; then
    wait "$web_pid"
    exit $?
  fi
  attempt=$((attempt + 1))
  if [ "$attempt" -ge 100 ]; then
    printf '%s\n' "Vite did not become ready at http://127.0.0.1:5173/" >&2
    exit 1
  fi
  sleep 0.1
done

"$tmp/log-leopard" \
  -addr 127.0.0.1:8787 \
  -browser-url http://127.0.0.1:5173/ \
  "$@" &
backend_pid=$!

while kill -0 "$backend_pid" 2>/dev/null && kill -0 "$web_pid" 2>/dev/null; do
  sleep 1
done

if ! kill -0 "$backend_pid" 2>/dev/null; then
  wait "$backend_pid"
else
  wait "$web_pid"
fi
