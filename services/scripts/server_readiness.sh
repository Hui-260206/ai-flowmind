#!/usr/bin/env bash
set -euo pipefail

services_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
env_file="$services_dir/.env"

if [[ ! -f "$env_file" ]]; then
  echo "missing services/.env; copy services/.env.example and configure a dedicated readiness target" >&2
  exit 1
fi

set -a
. "$env_file"
set +a

require() {
  local key=$1 value=${!1:-}
  if [[ -z "$value" ]]; then
    echo "missing required readiness configuration: $key" >&2
    exit 1
  fi
}
require FLOWMIND_READINESS
require MYSQL_DATABASE
require MYSQL_HOST
require MYSQL_USER
require MYSQL_PASSWORD
require REDIS_ADDR
require FLOWMIND_READINESS_REDIS_STOP_COMMAND
require FLOWMIND_READINESS_REDIS_START_COMMAND
require FLOWMIND_READINESS_MYSQL_STOP_COMMAND
require FLOWMIND_READINESS_MYSQL_START_COMMAND

if [[ "$FLOWMIND_READINESS" != "1" ]]; then
  echo "set FLOWMIND_READINESS=1 to explicitly authorize strict readiness verification" >&2
  exit 1
fi
if [[ "$MYSQL_DATABASE" != *_test ]]; then
  echo "MYSQL_DATABASE must end in _test for readiness verification; refusing $MYSQL_DATABASE" >&2
  exit 1
fi
if [[ "${REDIS_DB:-0}" == "0" ]]; then
  echo "REDIS_DB must be non-zero for readiness verification" >&2
  exit 1
fi

for tool in curl go uv mysql redis-cli shasum; do
  command -v "$tool" >/dev/null || { echo "missing required command: $tool" >&2; exit 1; }
done

run_id="readiness-$(date +%s)-$$"
api_port=${FLOWMIND_READINESS_HTTP_PORT:-18080}
grpc_port=${FLOWMIND_READINESS_GRPC_PORT:-15051}
api_addr="127.0.0.1:$api_port"
grpc_addr="127.0.0.1:$grpc_port"
log_dir=$(mktemp -d "${TMPDIR:-/tmp}/flowmind-readiness.XXXXXX")
python_pid=""
go_pid=""
redis_stopped=""
mysql_stopped=""
session=""
owner=""
redis_client_message_ids=(readiness-message python-down timeout-message cancelled-message redis-down)

cleanup() {
  local code=$?
  [[ -n "$go_pid" ]] && kill "$go_pid" 2>/dev/null || true
  [[ -n "$python_pid" ]] && kill "$python_pid" 2>/dev/null || true
  [[ -n "$go_pid" ]] && wait "$go_pid" 2>/dev/null || true
  [[ -n "$python_pid" ]] && wait "$python_pid" 2>/dev/null || true
	if [[ -n "$redis_stopped" ]]; then
		bash -lc "$FLOWMIND_READINESS_REDIS_START_COMMAND" || true
	fi
	if [[ -n "$mysql_stopped" ]]; then
		bash -lc "$FLOWMIND_READINESS_MYSQL_START_COMMAND" || true
	fi
	if [[ -n "$session" ]]; then
		MYSQL_PWD="$MYSQL_PASSWORD" mysql --protocol=TCP -h "$MYSQL_HOST" -P "${MYSQL_PORT:-3306}" -u "$MYSQL_USER" "$MYSQL_DATABASE" -e "DELETE FROM chat_messages WHERE session_id = '$session'; DELETE FROM chat_send_operations WHERE session_id = '$session'; DELETE FROM chat_sessions WHERE id = '$session';" || true
	fi
	if [[ -n "$owner" ]]; then
		for client_message_id in "${redis_client_message_ids[@]}"; do
			redis-cli -h "${REDIS_ADDR%:*}" -p "${REDIS_ADDR##*:}" -n "$REDIS_DB" ${REDIS_PASSWORD:+-a "$REDIS_PASSWORD"} --no-auth-warning DEL "idempotency:$(sha256 "$owner"):$(sha256 "$session"):$(sha256 "$client_message_id")" >/dev/null 2>&1 || true
		done
		redis-cli -h "${REDIS_ADDR%:*}" -p "${REDIS_ADDR##*:}" -n "$REDIS_DB" ${REDIS_PASSWORD:+-a "$REDIS_PASSWORD"} --no-auth-warning DEL "lock:conversation:$(sha256 "$session")" "rate_limit:owner:$(sha256 "$owner"):$(date -u +%Y%m%d%H%M)" >/dev/null 2>&1 || true
	fi
  if [[ $code -ne 0 ]]; then
    echo "readiness verification failed; logs retained in $log_dir" >&2
    [[ -f "$log_dir/go.log" ]] && tail -n 80 "$log_dir/go.log" >&2 || true
    [[ -f "$log_dir/python.log" ]] && tail -n 80 "$log_dir/python.log" >&2 || true
  else
    rm -rf "$log_dir"
  fi
}
trap cleanup EXIT INT TERM

wait_http() {
  local url=$1 expected=$2
  for _ in $(seq 1 80); do
    local status
    status=$(curl -sS -o /dev/null -w '%{http_code}' "$url" 2>/dev/null || true)
    [[ "$status" == "$expected" ]] && return 0
    sleep 0.15
  done
  echo "timed out waiting for $url to return $expected" >&2
  return 1
}

wait_mysql() {
  for _ in $(seq 1 120); do
    if MYSQL_PWD="$MYSQL_PASSWORD" mysql --protocol=TCP -h "$MYSQL_HOST" -P "${MYSQL_PORT:-3306}" -u "$MYSQL_USER" "$MYSQL_DATABASE" -e 'SELECT 1' >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.5
  done
  echo "timed out waiting for MySQL to accept readiness connections" >&2
  return 1
}

start_python() {
	(
		cd "$services_dir/ai-service"
		exec env AI_PROVIDER=fake AI_PROVIDER_BASE_URL= AI_PROVIDER_API_KEY= AI_GRPC_LISTEN_ADDR="$grpc_addr" AI_FAKE_PROVIDER_DELAY="${AI_FAKE_PROVIDER_DELAY:-0}" \
			uv run python -m app.main
	) >"$log_dir/python.log" 2>&1 &
  python_pid=$!
}
start_go() {
	(
		cd "$services_dir/go-api"
		go build -o "$log_dir/flowmind-go-api" ./cmd/api
	)
	env GO_HTTP_ADDR="$api_addr" \
		AI_GRPC_ADDR="$grpc_addr" \
		AI_GRPC_TIMEOUT="${AI_GRPC_TIMEOUT:-14s}" \
		GO_WRITE_TIMEOUT="${GO_WRITE_TIMEOUT:-16s}" \
		REDIS_SESSION_LOCK_TTL="${REDIS_SESSION_LOCK_TTL:-30s}" \
		AI_PROVIDER_TIMEOUT="${AI_PROVIDER_TIMEOUT:-12}" \
		"$log_dir/flowmind-go-api" >"$log_dir/go.log" 2>&1 &
  go_pid=$!
  wait_http "http://$api_addr/readyz" 200
}
stop_python() { [[ -n "$python_pid" ]] && kill "$python_pid" 2>/dev/null || true; [[ -n "$python_pid" ]] && wait "$python_pid" 2>/dev/null || true; python_pid=""; }
stop_go() { [[ -n "$go_pid" ]] && kill "$go_pid" 2>/dev/null || true; [[ -n "$go_pid" ]] && wait "$go_pid" 2>/dev/null || true; go_pid=""; }

json_field() { jq -er "$1"; }
sha256() { printf '%s' "$1" | shasum -a 256 | awk '{print $1}'; }
request() {
  local method=$1 path=$2 client=$3 request_id=$4 body=${5:-}
  local args=(-sS -X "$method" "http://$api_addr$path" -H "X-Client-ID: $client" -H "X-Request-ID: $request_id" -H 'Content-Type: application/json')
  [[ -n "$body" ]] && args+=(--data "$body")
  curl "${args[@]}"
}
assert_code() {
  local expected=$1 actual=$2
  [[ "$actual" == "$expected" ]] || { echo "expected HTTP $expected, got $actual" >&2; return 1; }
}

start_python
start_go

suffix=$(printf '%012x' "$(( ( $(date +%s) + $$ ) % 281474976710656 ))")
owner="123e4567-e89b-12d3-a456-$suffix"
other="123e4567-e89b-12d3-a456-426614174099"
create=$(request POST /api/v1/sessions "$owner" "req-$run_id-create")
session=$(printf '%s' "$create" | json_field '.id')
send_body='{"content":"readiness message","client_message_id":"readiness-message"}'
send=$(request POST "/api/v1/sessions/$session/messages" "$owner" "req-$run_id-send" "$send_body")
printf '%s' "$send" | json_field '.request_id' | grep -qx "req-$run_id-send"
grep -q "req-$run_id-send" "$log_dir/python.log"
user_id=$(printf '%s' "$send" | json_field '.user_message.id')
assistant_id=$(printf '%s' "$send" | json_field '.assistant_message.id')
printf '%s' "$send" | json_field '.assistant_message.content' | grep -qx 'This is a fixed response from the FlowMind fake provider.'

history=$(request GET "/api/v1/sessions/$session/messages" "$owner" "req-$run_id-history")
[[ $(printf '%s' "$history" | jq '.items | length') == 2 ]]
[[ $(printf '%s' "$history" | json_field '.items[0].id') == "$user_id" ]]
[[ $(printf '%s' "$history" | json_field '.items[1].id') == "$assistant_id" ]]

replay=$(request POST "/api/v1/sessions/$session/messages" "$owner" "req-$run_id-replay" "$send_body")
[[ $(printf '%s' "$replay" | json_field '.user_message.id') == "$user_id" ]]
[[ $(printf '%s' "$replay" | json_field '.assistant_message.id') == "$assistant_id" ]]

cross_status=$(curl -sS -o "$log_dir/cross.json" -w '%{http_code}' "http://$api_addr/api/v1/sessions/$session/messages" -H "X-Client-ID: $other")
assert_code 404 "$cross_status"
[[ $(jq -r '.error.code' "$log_dir/cross.json") == SESSION_NOT_FOUND ]]

stop_go
start_go
history_after_restart=$(request GET "/api/v1/sessions/$session/messages" "$owner" "req-$run_id-restart")
[[ $(printf '%s' "$history_after_restart" | jq '.items | length') == 2 ]]

stop_python
wait_http "http://$api_addr/readyz" 503
python_down=$(curl -sS -o "$log_dir/python-down.json" -w '%{http_code}' -X POST "http://$api_addr/api/v1/sessions/$session/messages" -H "X-Client-ID: $owner" -H 'Content-Type: application/json' --data '{"content":"python unavailable","client_message_id":"python-down"}')
assert_code 502 "$python_down"
[[ $(jq -r '.error.code' "$log_dir/python-down.json") == AI_UNAVAILABLE ]]

# Restart with a deliberately slow Fake Provider and a bounded Go RPC timeout.
stop_go
AI_FAKE_PROVIDER_DELAY=2 start_python
AI_GRPC_TIMEOUT=1s GO_WRITE_TIMEOUT=2s REDIS_SESSION_LOCK_TTL=3s AI_PROVIDER_TIMEOUT=0.5 start_go
timeout_status=$(curl -sS -o "$log_dir/timeout.json" -w '%{http_code}' -X POST "http://$api_addr/api/v1/sessions/$session/messages" -H "X-Client-ID: $owner" -H 'Content-Type: application/json' --data '{"content":"timeout","client_message_id":"timeout-message"}')
assert_code 504 "$timeout_status"
[[ $(jq -r '.error.code' "$log_dir/timeout.json") == AI_TIMEOUT ]]

# A client disconnect must cancel the Go request context and its gRPC child
# call. The durable user message may already exist, but no assistant response
# may be written after the caller has gone away.
history_before_cancel=$(request GET "/api/v1/sessions/$session/messages" "$owner" "req-$run_id-before-cancel")
message_count_before_cancel=$(printf '%s' "$history_before_cancel" | jq '.items | length')
cancel_exit=0
curl -sS --max-time 0.2 -X POST "http://$api_addr/api/v1/sessions/$session/messages" -H "X-Client-ID: $owner" -H 'Content-Type: application/json' --data '{"content":"cancelled","client_message_id":"cancelled-message"}' >/dev/null || cancel_exit=$?
[[ "$cancel_exit" == 28 ]]
sleep 0.3
history_after_cancel=$(request GET "/api/v1/sessions/$session/messages" "$owner" "req-$run_id-after-cancel")
[[ $(printf '%s' "$history_after_cancel" | jq '.items | length') == $((message_count_before_cancel + 1)) ]]
[[ $(printf '%s' "$history_after_cancel" | jq -r '.items[-1].role') == user ]]
[[ $(printf '%s' "$history_after_cancel" | jq -r '.items[-1].client_message_id') == cancelled-message ]]

stop_go
stop_python
unset AI_FAKE_PROVIDER_DELAY
start_python
start_go

# Commands are explicitly supplied by the operator for the dedicated Redis
# readiness instance. The existing Go connection must then fail closed for new
# sends while MySQL-backed reads remain available.
bash -lc "$FLOWMIND_READINESS_REDIS_STOP_COMMAND"
redis_stopped=1
redis_status=$(curl -sS -o "$log_dir/redis-down.json" -w '%{http_code}' -X POST "http://$api_addr/api/v1/sessions/$session/messages" -H "X-Client-ID: $owner" -H 'Content-Type: application/json' --data '{"content":"redis unavailable","client_message_id":"redis-down"}')
assert_code 503 "$redis_status"
[[ $(jq -r '.error.code' "$log_dir/redis-down.json") == REDIS_UNAVAILABLE ]]
history_during_redis_outage=$(request GET "/api/v1/sessions/$session/messages" "$owner" "req-$run_id-redis-history")
[[ $(printf '%s' "$history_during_redis_outage" | jq '.items | length') == 2 ]]
bash -lc "$FLOWMIND_READINESS_REDIS_START_COMMAND"
redis_stopped=""

# The host command pair is explicitly supplied by the operator and targets
# only the dedicated local MySQL service. A failed readiness check must not
# silently mask a database outage; cleanup restores it even on assertion
# failure.
bash -lc "$FLOWMIND_READINESS_MYSQL_STOP_COMMAND"
mysql_stopped=1
wait_http "http://$api_addr/readyz" 503
mysql_status=$(curl -sS -o "$log_dir/mysql-down.json" -w '%{http_code}' "http://$api_addr/api/v1/sessions" -H "X-Client-ID: $owner")
assert_code 500 "$mysql_status"
[[ $(jq -r '.error.code' "$log_dir/mysql-down.json") == INTERNAL_ERROR ]]
bash -lc "$FLOWMIND_READINESS_MYSQL_START_COMMAND"
mysql_stopped=""
wait_mysql
wait_http "http://$api_addr/readyz" 200

echo "server readiness verification passed ($run_id)"
