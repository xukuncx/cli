#!/usr/bin/env bash
# 一键 bootstrap：起 mock + 注入 scope + 准备本地 Agent E2E 测试环境。
#
# 用法：
#   ./scripts/apps-test-env.sh                  # success 模式
#   ./scripts/apps-test-env.sh build_failed     # 切到失败 mode
#   ./scripts/apps-test-env.sh stop             # 停 mock
#
# 脚本退出时会保留 mock 在后台跑（PID 写到 /tmp/apps-mock.pid）。
# 启动 Claude Code 时请显式带 env：
#   LARK_CLI_OPEN_API_BASE=http://127.0.0.1:8181 claude
#
# 详细背景见 docs/dev/miaoda-local-dev.md 的「开发者本地 Agent 测试」章。

set -euo pipefail
cd "$(dirname "$0")/.."   # repo root

MOCK_PORT=8181
PIDFILE=/tmp/apps-mock.pid
MODE="${1:-success}"

# ---- stop ----
if [ "$MODE" = "stop" ]; then
  if [ -f "$PIDFILE" ]; then
    kill "$(cat $PIDFILE)" 2>/dev/null || true
    rm -f "$PIDFILE"
  fi
  lsof -ti:$MOCK_PORT 2>/dev/null | xargs kill -9 2>/dev/null || true
  echo "mock stopped"
  exit 0
fi

# ---- check mode ----
case "$MODE" in
  success|build_failed|app_not_found|http_503) ;;
  *) echo "unknown mode '$MODE' (expected: success | build_failed | app_not_found | http_503 | stop)" >&2; exit 2 ;;
esac

# ---- (1) clean port + start mock ----
echo "[1/3] (re)starting mock in '$MODE' mode on 127.0.0.1:$MOCK_PORT"
lsof -ti:$MOCK_PORT 2>/dev/null | xargs kill -9 2>/dev/null || true
sleep 2   # 让 socket 真正释放（不等会撞 CLOSED 半开态）

python3 scripts/apps-mock.py "$MODE" >/tmp/apps-mock.log 2>&1 &
echo $! > "$PIDFILE"

# 轮询等端口可连（最长 15 秒）。socket 状态在 mac 上 CLOSED→LISTEN 有时要好几秒。
ready=false
for _ in $(seq 1 30); do
  if nc -z 127.0.0.1 "$MOCK_PORT" 2>/dev/null; then
    ready=true; break
  fi
  sleep 0.5
done
if [ "$ready" != true ]; then
  echo "ERR: mock failed to start listening on $MOCK_PORT within 15s. log:" >&2
  cat /tmp/apps-mock.log >&2
  exit 1
fi
echo "    mock listening (PID=$(cat $PIDFILE), log=/tmp/apps-mock.log)"

# ---- (2) read appId / userOpenId from ~/.lark-cli/config.json ----
echo "[2/3] reading active appId / userOpenId from ~/.lark-cli/config.json"
if ! command -v jq >/dev/null 2>&1; then
  echo "ERR: jq not found (brew install jq)" >&2; exit 1
fi
APPID=$(jq -r '.apps[0].appId // empty' ~/.lark-cli/config.json)
USERID=$(jq -r '.apps[0].users[0].userOpenId // empty' ~/.lark-cli/config.json)
if [ -z "$APPID" ] || [ -z "$USERID" ]; then
  echo "ERR: no appId / userOpenId in ~/.lark-cli/config.json (run 'lark-cli config init' + 'lark-cli auth login --recommend' first)" >&2
  exit 1
fi
echo "    appId=$APPID  userOpenId=$USERID"

# ---- (3) inject spark:* scopes into stored UAT ----
# 跟生产代码（shortcuts/apps/*.go 里的 Scopes 字段）保持一致：BOE 后端目前在
# spark 命名空间下注册了 apps 域的所有 OAPI（详见 common.go:10 注释）。
# 待 miaoda 命名空间注册稳定后这里和 *.go 一起切回 miaoda:app:* 即可。
echo "[3/3] injecting spark:app:* scopes into stored UAT"
go run ./cmd/_apps_dev_tools/inject_scopes "$APPID" "$USERID" \
  spark:app:write spark:app:read spark:app:publish \
  spark:app.access_scope:read spark:app.access_scope:write

cat <<EOF

=== ready ===
Mock OAPI server is running. Next step: launch Claude Code with the env override.

    LARK_CLI_OPEN_API_BASE=http://127.0.0.1:$MOCK_PORT claude

Inside Claude Code, try the test plan §5.2 D-prompts (e.g. "创建一个妙搭应用叫 demo").

To switch mock failure mode (build_failed / app_not_found / http_503):
    ./scripts/apps-test-env.sh build_failed

To stop mock:
    ./scripts/apps-test-env.sh stop

Note: every time lark-cli refreshes the UAT (server-side), the injected scopes
are overwritten by the server-provided scope list. Re-run this script if you
see missing_scope errors again.
EOF
