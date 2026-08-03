#!/usr/bin/env sh
set -eu

qa_api_url="${QA_API_URL:-}"
qa_app_url="${QA_APP_URL:-}"
qa_email="${QA_USER_EMAIL:-qa_user@qa.treechat.test}"
qa_password="${QA_USER_PASSWORD:-}"
qa_allow_nonisolated="${QA_ALLOW_NONISOLATED:-0}"

if [ -z "$qa_api_url" ]; then
    echo "QA_API_URL is required" >&2
    exit 2
fi
if [ -z "$qa_password" ]; then
    echo "QA_USER_PASSWORD is required" >&2
    exit 2
fi

case "$qa_api_url" in
    *molt-bot.tail206f1c.ts.net*|*bill-vps.tail206f1c.ts.net*|http://localhost:*|http://127.0.0.1:*) ;;
    *)
        if [ "$qa_allow_nonisolated" != "1" ]; then
            echo "refusing non-isolated QA host: $qa_api_url" >&2
            echo "set QA_ALLOW_NONISOLATED=1 only when this is intentional" >&2
            exit 2
        fi
        ;;
esac

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(dirname "$script_dir")
qa_tmp_dir=$(mktemp -d)
trap 'find "$qa_tmp_dir" -depth -delete' EXIT INT TERM

qa_binary="${TREECLI_BINARY:-}"
if [ -z "$qa_binary" ]; then
    qa_binary="${qa_tmp_dir}/treecli-smoke"
    (cd "$repo_root" && go build -trimpath -o "$qa_binary" .)
fi

export XDG_CONFIG_HOME="${qa_tmp_dir}/config"
export TREECLI_ALLOW_INSECURE_HTTP=0
export TREECTL_ALLOW_INSECURE_HTTP=0

if command -v curl >/dev/null 2>&1; then
    curl -fsS "${qa_api_url%/}/health" >/dev/null
fi

run_cli() {
    if [ -n "$qa_app_url" ]; then
        "$qa_binary" \
            --profile release-smoke \
            --backend-url "$qa_api_url" \
            --app-host "$qa_app_url" \
            "$@"
        return
    fi

    "$qa_binary" \
        --profile release-smoke \
        --backend-url "$qa_api_url" \
        "$@"
}

printf '%s\n' "$qa_password" | run_cli login --email "$qa_email" --password-stdin

profile_json="${qa_tmp_dir}/profile.json"
run_cli profile show --json > "$profile_json"
if grep -F "$qa_email" "$profile_json" >/dev/null 2>&1; then
    echo "profile output leaked the QA email" >&2
    exit 1
fi
space_id=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1])).get("active_space_id", ""))' "$profile_json")
if [ -z "$space_id" ]; then
    echo "QA user bootstrap did not provide an active space" >&2
    exit 1
fi

notifications_json="${qa_tmp_dir}/notifications.json"
run_cli get notifications --space-id "$space_id" --json > "$notifications_json"
python3 -m json.tool "$notifications_json" >/dev/null

if run_cli get upvalues --help >/dev/null 2>&1; then
    upvalues_json="${qa_tmp_dir}/upvalues.json"
    run_cli get upvalues --per-page 5 --json > "$upvalues_json"
    python3 -m json.tool "$upvalues_json" >/dev/null
fi

smoke_marker="treecli-release-smoke-$(date -u +%Y%m%dT%H%M%SZ)"
created_json="${qa_tmp_dir}/created.json"
run_cli new post "$smoke_marker" --private --json > "$created_json"
quest_id=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["quest"]["id"])' "$created_json")
answer_id=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["quest"]["parent"]["id"])' "$created_json")

thread_json="${qa_tmp_dir}/thread.json"
run_cli get thread "$quest_id" --json > "$thread_json"
python3 -m json.tool "$thread_json" >/dev/null
grep -F "$smoke_marker" "$thread_json" >/dev/null

message_json="${qa_tmp_dir}/message.json"
run_cli get messages "$answer_id" --json > "$message_json"
python3 -m json.tool "$message_json" >/dev/null
grep -F "$smoke_marker" "$message_json" >/dev/null

echo "QA smoke passed"
echo "API: ${qa_api_url}"
echo "Thread: ${quest_id}"
if [ -n "$qa_app_url" ]; then
    echo "Thread URL: ${qa_app_url%/}/quest/${quest_id}"
fi
