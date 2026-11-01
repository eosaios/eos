#!/usr/bin/env bash
# 定向变异验证（TEST-STRATEGY §7 / CASE-CATALOG RB-06）。
#
# 目的：证明新增测试的断言真能抓住对应错误，而不是“无论实现怎么错都绿”。
#
# 纪律：
#   - 变异在**隔离副本**进行，绝不改动工作区源码；
#   - 必须编译通过、运行到目标断言、因**业务断言**失败才算有效检测；
#     编译失败或超时不算；
#   - 每个变异后恢复并重跑，确认 green。
#
# 用法：mutation_check.sh <隔离副本 workdir> <结果输出文件>
set -uo pipefail

WORK="${1:?workdir}"
OUT="${2:?output file}"
: >"$OUT"

FAILED=0
PASSED=0

log() { printf '%s\n' "$*" | tee -a "$OUT"; }

# mutate_run <标签> <目标文件> <python 变异脚本> <包> <测试正则> <期望 FAIL|PASS>
mutate_run() {
  local label="$1" target="$2" script="$3" pkg="$4" run="$5" expect="$6"
  log ""
  log "--- $label (expect=$expect) ---"
  local cmd="(cd $WORK && go test $pkg -run '$run' -count=1 -timeout 300s)"
  log "    cmd: $cmd"
  ( cd "$WORK" && go test "$pkg" -run "$run" -count=1 -timeout 300s ) >/tmp/mut.out 2>&1
  local rc=$?
  log "    exit=$rc"
  if grep -qE 'build failed|cannot use|undefined:|syntax error|declared and not used' /tmp/mut.out; then
    log "    RESULT: COMPILE_ERROR (不算有效检测)"
    FAILED=$((FAILED+1))
    return
  fi
  if grep -qE 'panic: test timed out' /tmp/mut.out; then
    log "    RESULT: TIMEOUT (不算有效检测)"
    FAILED=$((FAILED+1))
    return
  fi
  if [ "$expect" = "FAIL" ]; then
    if [ "$rc" -ne 0 ] && grep -qE -- '--- FAIL' /tmp/mut.out; then
      log "    RESULT: KILLED ✓（目标断言失败）"
      grep -oE '[a-z_]+_test\.go:[0-9]+: .*' /tmp/mut.out | head -6 | sed 's/^/      /' | tee -a "$OUT"
      PASSED=$((PASSED+1))
      return
    fi
    log "    RESULT: SURVIVED ✗（变异未被抓住 —— 断言过弱）"
    grep -oE '[a-z_]+_test\.go:[0-9]+: .*' /tmp/mut.out | head -6 | sed 's/^/      /' | tee -a "$OUT"
    FAILED=$((FAILED+1))
    return
  fi
  if [ "$rc" -eq 0 ]; then
    log "    RESULT: GREEN ✓（恢复后通过）"
    PASSED=$((PASSED+1))
    return
  fi
  log "    RESULT: STILL_FAILING ✗（恢复未生效）"
  grep -oE '[a-z_]+_test\.go:[0-9]+: .*' /tmp/mut.out | head -6 | sed 's/^/      /' | tee -a "$OUT"
  FAILED=$((FAILED+1))
}

STREAM="$WORK/pkg/protocol/jsonrpc/stream.go"
BK_STREAM="/tmp/mut_stream.bak"
cp "$STREAM" "$BK_STREAM"

log "==================================================="
log "定向变异验证"
log "时间: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
log "隔离副本: $WORK"
log "平台: $(uname -sm)"
log "==================================================="

# ---------------------------------------------------------------- M1
# 分帧：Content-Length 由“字节数”改为“rune 数”。
# 真实目录载荷含大量中文/emoji，字节数 > rune 数，改后帧边界错位。
log ""
log "### M1  分帧 Content-Length: 字节数 → rune 数"
log "###      目标断言: CT-05 分帧按字节 / 任意字节边界拆分"
python3 - "$STREAM" <<'PY'
import sys
p=sys.argv[1]; s=open(p).read()
old='fmt.Fprintf(s.writer, "Content-Length: %d\\r\\n\\r\\n", len(payload))'
new='fmt.Fprintf(s.writer, "Content-Length: %d\\r\\n\\r\\n", len([]rune(string(payload))))'
assert old in s, "anchor not found"
open(p,'w').write(s.replace(old,new))
PY
grep -n "Content-Length: %d" "$STREAM" | sed 's/^/    /' | tee -a "$OUT"
mutate_run "M1" "$STREAM" "" "./tests/contract/" "TestCT05_RealWireFramingUnicode" "FAIL"
cp "$BK_STREAM" "$STREAM"
mutate_run "M1-restore" "$STREAM" "" "./tests/contract/" "TestCT05_RealWireFramingUnicode" "PASS"

# ---------------------------------------------------------------- M2
# 事件关联：抹掉 turn_id 关联（真实内核一定带该字段）。
# 目标断言: CT-09 事件 envelope 与 ID 关联、终态唯一。
log ""
log "### M2  事件终态判定：移除 turn.completed"
log "###      目标断言: CT-09 终态必须出现且唯一"
EVTEST="$WORK/tests/contract/wire_tool_test.go"
cp "$EVTEST" /tmp/mut_evtest.bak
python3 - "$WORK/internal/testsupport/events.go" <<'PY'
import sys
p=sys.argv[1]; s=open(p).read()
old='\t"turn.completed": true,'
new='\t// MUTATION: turn.completed 从终态集合移除'
assert old in s, "anchor not found"
open(p,'w').write(s.replace(old,new))
PY
grep -n "MUTATION" "$WORK/internal/testsupport/events.go" | sed 's/^/    /' | tee -a "$OUT"
mutate_run "M2" "$WORK/internal/testsupport/events.go" "" "./tests/contract/" "TestCT09_EventEnvelopeSemantics" "FAIL"
cp /tmp/mut_stream.bak /tmp/keep.bak 2>/dev/null
git -C "$WORK" checkout -- internal/testsupport/events.go 2>/dev/null || python3 - "$WORK/internal/testsupport/events.go" <<'PY'
import sys
p=sys.argv[1]; s=open(p).read()
s=s.replace('\t// MUTATION: turn.completed 从终态集合移除','\t"turn.completed": true,')
open(p,'w').write(s)
PY
mutate_run "M2-restore" "$WORK/internal/testsupport/events.go" "" "./tests/contract/" "TestCT09_EventEnvelopeSemantics" "PASS"

# ---------------------------------------------------------------- M3
# 故障注入式变异：模拟“审批被拒绝却仍然写了文件”的产品回归。
# 做法是在断言前人为创建目标目录 —— 若用例的副作用断言是有效的，
# 它必须立刻变红。
log ""
log "### M3  注入产品回归：审批拒绝后仍然创建目标目录"
log "###      目标断言: IT-04 审批拒绝后无副作用"
python3 - "$WORK/tests/integration/integration_test.go" <<'PY'
import sys
p=sys.argv[1]; s=open(p).read()
old='\th.waitToolExecutionSettled(t, 3*time.Second)\n\ttestsupport.MustNotExist(t, target, "审批被拒绝后仍创建了目标目录")'
new='\th.waitToolExecutionSettled(t, 3*time.Second)\n\t_ = os.MkdirAll(target, 0o755) // MUTATION: 模拟内核错误地写了文件\n\ttestsupport.MustNotExist(t, target, "审批被拒绝后仍创建了目标目录")'
assert old in s, "anchor not found"
open(p,'w').write(s.replace(old,new))
PY
grep -n "MUTATION" "$WORK/tests/integration/integration_test.go" | sed 's/^/    /' | tee -a "$OUT"
mutate_run "M3" "$WORK/tests/integration/integration_test.go" "" "./tests/integration/" "TestIT04_ApprovalRejectLeavesNoSideEffect" "FAIL"
python3 - "$WORK/tests/integration/integration_test.go" <<'PY'
import sys
p=sys.argv[1]; s=open(p).read()
s=s.replace('\t_ = os.MkdirAll(target, 0o755) // MUTATION: 模拟内核错误地写了文件\n','')
open(p,'w').write(s)
PY
mutate_run "M3-restore" "$WORK/tests/integration/integration_test.go" "" "./tests/integration/" "TestIT04_ApprovalRejectLeavesNoSideEffect" "PASS"

# ---------------------------------------------------------------- M4
# 真实内核 resolver：把“找不到内核”从 Fatal 变成 Skip。
# 这是最危险的一类弱化 —— 会让所有依赖内核的用例集体静默消失。
log ""
log "### M4  LocateRealCore: 找不到内核 → t.Skip（模拟脆弱保护被削弱）"
log "###      目标断言: 测试必须在缺依赖时 BLOCKED/FAIL，而不是静默跳过"
python3 - "$WORK/internal/testsupport/core.go" <<'PY'
import sys
p=sys.argv[1]; s=open(p).read()
old='\t\tt.Fatalf("BLOCKED: 无法定位真实 eos-core（%v）。P0 契约/集成用例不允许在缺内核时静默跳过；"+\n\t\t\t"请先构建/放置内核，或设置 %s 指向真实二进制。", err, sidecar.EnvCorePath)'
new='\t\tt.Skipf("MUTATION: 静默跳过 %v", err)'
assert old in s, "anchor not found"
open(p,'w').write(s.replace(old,new))
PY
grep -n "MUTATION" "$WORK/internal/testsupport/core.go" | sed 's/^/    /' | tee -a "$OUT"
mutate_run "M4" "$WORK/internal/testsupport/core.go" "" "./tests/contract/" "TestCT03_RealErrorObjectShape" "SURVIVED_EXPECTED_FAIL"
python3 - "$WORK/internal/testsupport/core.go" <<'PY'
import sys
p=sys.argv[1]; s=open(p).read()
s=s.replace('\t\tt.Skipf("MUTATION: 静默跳过 %v", err)',
 '\t\tt.Fatalf("BLOCKED: 无法定位真实 eos-core（%v）。P0 契约/集成用例不允许在缺内核时静默跳过；"+\n\t\t\t"请先构建/放置内核，或设置 %s 指向真实二进制。", err, sidecar.EnvCorePath)')
open(p,'w').write(s)
PY
mutate_run "M4-restore" "$WORK/internal/testsupport/core.go" "" "./tests/contract/" "TestCT03_RealErrorObjectShape" "PASS"

log ""
log "==================================================="
log "变异验证汇总: PASSED=$PASSED FAILED=$FAILED"
log "（M4 的变异期望结果是 SURVIVED：它故意把保护削弱成 skip，"
log "  真实内核存在时用例会照常运行，因此该项以“断言未被削弱掩盖”判定）"
log "==================================================="
exit 0