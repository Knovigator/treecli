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

run_cli_for_profile() {
    qa_profile="$1"
    shift
    if [ -n "$qa_app_url" ]; then
        "$qa_binary" \
            --env release-qa --account "$qa_profile" \
            --backend-url "$qa_api_url" \
            --app-host "$qa_app_url" \
            "$@"
        return
    fi

    "$qa_binary" \
        --env release-qa --account "$qa_profile" \
        --backend-url "$qa_api_url" \
        "$@"
}

run_cli() {
    run_cli_for_profile release-smoke "$@"
}

signup_timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
signup_email="treecli-smoke-${signup_timestamp}-$$@qa.treechat.test"
signup_username="treecli_smoke_${signup_timestamp}_$$"
printf '%s\n' "$qa_password" | run_cli_for_profile release-signup \
    signup --username "$signup_username" --email "$signup_email" --password-stdin

signup_profile_json="${qa_tmp_dir}/signup-profile.json"
run_cli_for_profile release-signup account show --json > "$signup_profile_json"
python3 -m json.tool "$signup_profile_json" >/dev/null
if grep -F "$signup_email" "$signup_profile_json" >/dev/null 2>&1; then
    echo "signup profile output leaked the disposable QA email" >&2
    exit 1
fi
signup_space_id=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1])).get("active_space_id", ""))' "$signup_profile_json")
if [ -z "$signup_space_id" ]; then
    echo "QA signup bootstrap did not provide an active space" >&2
    exit 1
fi

printf '%s\n' "$qa_password" | run_cli login --email "$qa_email" --password-stdin

profile_json="${qa_tmp_dir}/profile.json"
run_cli account show --json > "$profile_json"
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
run_cli_for_profile release-signup new post "$smoke_marker" --private --json > "$created_json"
quest_id=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["quest"]["id"])' "$created_json")
answer_id=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["quest"]["parent"]["id"])' "$created_json")

thread_json="${qa_tmp_dir}/thread.json"
run_cli_for_profile release-signup get threads "$quest_id" --json > "$thread_json"
python3 -m json.tool "$thread_json" >/dev/null
grep -F "$smoke_marker" "$thread_json" >/dev/null

message_json="${qa_tmp_dir}/message.json"
run_cli_for_profile release-signup get messages "$answer_id" --json > "$message_json"
python3 -m json.tool "$message_json" >/dev/null
grep -F "$smoke_marker" "$message_json" >/dev/null

# Use the disposable signup account so concurrent smoke runs cannot change
# which thread is newest for this author.
latest_json="${qa_tmp_dir}/latest.json"
run_cli_for_profile release-signup get threads --user me --root --limit 1 --json > "$latest_json"
python3 - "$latest_json" "$quest_id" <<'PYTEST'
import json, sys
result = json.load(open(sys.argv[1]))
assert [thread["id"] for thread in result["threads"]] == [sys.argv[2]], result
assert result["pagination"]["limit"] == 1, result
PYTEST

children_json="${qa_tmp_dir}/children.json"
run_cli_for_profile release-signup get threads --answer "$answer_id" --json > "$children_json"
python3 - "$children_json" "$quest_id" <<'PYTEST'
import json, sys
result = json.load(open(sys.argv[1]))
assert sys.argv[2] in [thread["id"] for thread in result["threads"]], result
PYTEST

# Exercise all explicit reply modes and verify persisted edges after reading back.
branch_json="${qa_tmp_dir}/branch-reply.json"
plain_json="${qa_tmp_dir}/branch-plain.json"
quote_json="${qa_tmp_dir}/quote-reply.json"
run_cli_for_profile release-signup branch-reply "$answer_id" "${smoke_marker}-branch" --json > "$branch_json"
run_cli_for_profile release-signup branch-reply "$answer_id" --no-quote "${smoke_marker}-plain" --json > "$plain_json"
branch_answer_id=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["answer"]["id"])' "$branch_json")
plain_answer_id=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["answer"]["id"])' "$plain_json")
run_cli_for_profile release-signup quote-reply "$branch_answer_id" "${smoke_marker}-quote" --json > "$quote_json"
quote_answer_id=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["answer"]["id"])' "$quote_json")
replies_json="${qa_tmp_dir}/replies.json"
run_cli_for_profile release-signup get messages "$branch_answer_id" "$plain_answer_id" "$quote_answer_id" --json > "$replies_json"
python3 - "$replies_json" "$quest_id" "$answer_id" "$branch_answer_id" "$plain_answer_id" "$quote_answer_id" <<'PYTEST'
import json, sys
posts = {post["id"]: post for post in json.load(open(sys.argv[1]))["answers"]}
for post_id, target in [(sys.argv[4], sys.argv[3]), (sys.argv[5], None), (sys.argv[6], sys.argv[4])]:
    post = posts[post_id]
    assert post["quest_id"] == sys.argv[2], post
    assert post.get("reply_to_answer_id") == target, post
PYTEST

echo "QA smoke passed"
echo "API: ${qa_api_url}"
echo "Thread: ${quest_id}"
if [ -n "$qa_app_url" ]; then
    echo "Thread URL: ${qa_app_url%/}/quest/${quest_id}"
fi
