#!/usr/bin/env bash
# test-sse.sh — End-to-end SSE integration validation
# Usage: PORT=61017 bash scripts/test-sse.sh
#
# Requires: daemon binary running, curl, jq

set -uo pipefail

PORT="${PORT:-61017}"
BASE_URL="http://localhost:${PORT}"

PASS=0
FAIL=0

# ── helpers ───────────────────────────────────────────────────────────────────

pass() { echo "  ✓ $1"; PASS=$((PASS + 1)); }
fail() { echo "  ✗ $1"; FAIL=$((FAIL + 1)); }

require_cmd() {
    if ! command -v "$1" &>/dev/null; then
        echo "ERROR: required command '$1' not found" >&2
        exit 1
    fi
}

# ── prerequisites ─────────────────────────────────────────────────────────────

require_cmd curl
require_cmd jq

# ── Check 1: Daemon health check ──────────────────────────────────────────────

echo "Check 1: Daemon health check"
HEALTH_OK=false
for i in $(seq 1 20); do
    if curl -sf "${BASE_URL}/api/status" >/dev/null 2>&1; then
        HEALTH_OK=true
        break
    fi
    sleep 0.5
done

if [ "$HEALTH_OK" = "true" ]; then
    pass "Daemon is healthy at ${BASE_URL}"
else
    fail "Daemon did not respond within 10s at ${BASE_URL}"
    echo ""
    echo "RESULTS: ${PASS} passed, ${FAIL} failed"
    exit 1
fi

# ── Check 2: Create a slow shell job ─────────────────────────────────────────

echo "Check 2: Create shell job"
JOB_RESPONSE=$(curl -sf -X POST "${BASE_URL}/api/jobs" \
    -H "Content-Type: application/json" \
    -d '{"name":"sse-integration-test","executor":"shell","trigger":{"type":"once"},"enabled":true,"shell":{"command":"for i in 1 2 3 4 5; do echo \"step $i\"; sleep 0.3; done"}}' 2>&1)

JOB_ID=$(echo "$JOB_RESPONSE" | jq -r '.id // empty' 2>/dev/null)
if [ -n "$JOB_ID" ]; then
    pass "Job created with ID: ${JOB_ID}"
else
    fail "Failed to create job: ${JOB_RESPONSE}"
    echo ""
    echo "RESULTS: ${PASS} passed, ${FAIL} failed"
    exit 1
fi

# ── Check 3: Trigger the job ──────────────────────────────────────────────────

echo "Check 3: Trigger job"
TRIGGER_RESPONSE=$(curl -sf -X POST "${BASE_URL}/api/jobs/${JOB_ID}/trigger" 2>&1)
TRIGGER_STATUS=$(echo "$TRIGGER_RESPONSE" | jq -r '.status // empty' 2>/dev/null)
if [ "$TRIGGER_STATUS" = "triggered" ]; then
    pass "Job triggered successfully"
else
    fail "Failed to trigger job: ${TRIGGER_RESPONSE}"
fi

# ── Check 4: Find the run ID by polling ──────────────────────────────────────

echo "Check 4: Find run ID"
RUN_ID=""
for i in $(seq 1 10); do
    RUNS_JSON=$(curl -sf "${BASE_URL}/api/runs?limit=5" 2>/dev/null || echo "[]")
    RUN_ID=$(echo "$RUNS_JSON" | jq -r --arg jid "$JOB_ID" \
        '[.[] | select(.jobId == $jid)] | first | .id // empty' 2>/dev/null)
    if [ -n "$RUN_ID" ]; then
        break
    fi
    sleep 0.5
done

if [ -n "$RUN_ID" ]; then
    pass "Found run ID: ${RUN_ID}"
else
    fail "Could not find run for job ${JOB_ID} after 10 attempts"
    curl -sf -X DELETE "${BASE_URL}/api/jobs/${JOB_ID}" >/dev/null 2>&1 || true
    echo ""
    echo "RESULTS: ${PASS} passed, ${FAIL} failed"
    exit 1
fi

# ── Subscribe to SSE stream (operation) ───────────────────────────────────────

echo "Subscribing to SSE stream..."
SSE_OUTPUT=$(curl -sf -N --max-time 15 "${BASE_URL}/api/runs/${RUN_ID}/stream" 2>&1 || true)

# ── Check 5: Verify at least one chunk event ──────────────────────────────────

echo "Check 5: Verify chunk events in SSE output"
if echo "$SSE_OUTPUT" | grep -q '"chunk"'; then
    pass "SSE output contains chunk events"
else
    fail "SSE output missing chunk events"
fi

# ── Check 6: Verify chunk content contains step output ───────────────────────

echo "Check 6: Verify chunk content contains step output"
if echo "$SSE_OUTPUT" | grep -q 'step '; then
    pass "Chunk content contains expected 'step N' output"
else
    fail "Chunk content missing expected 'step N' output"
fi

# ── Check 7: Verify 'event: done' line ───────────────────────────────────────

echo "Check 7: Verify 'event: done' in SSE output"
if echo "$SSE_OUTPUT" | grep -q "^event: done"; then
    pass "SSE output contains 'event: done'"
else
    fail "SSE output missing 'event: done'"
fi

# ── Extract done payload ──────────────────────────────────────────────────────

DONE_DATA=$(echo "$SSE_OUTPUT" | grep -A1 "^event: done" | grep "^data:" | sed 's/^data: //')

# ── Check 8: Verify done payload status == success ───────────────────────────

echo "Check 8: Verify done payload status == success"
DONE_STATUS=$(echo "$DONE_DATA" | jq -r '.status // empty' 2>/dev/null)
if [ "$DONE_STATUS" = "success" ]; then
    pass "Done payload status is 'success'"
else
    fail "Done payload status is '${DONE_STATUS}', expected 'success'"
fi

# ── Check 9: Verify done payload has started_at ──────────────────────────────

echo "Check 9: Verify done payload has started_at"
DONE_STARTED=$(echo "$DONE_DATA" | jq -r '.started_at // empty' 2>/dev/null)
if [ -n "$DONE_STARTED" ]; then
    pass "Done payload has started_at: ${DONE_STARTED}"
else
    fail "Done payload missing started_at timestamp"
fi

# ── Check 10: Verify completed run replay ────────────────────────────────────

echo "Check 10: Verify completed run replay"
REPLAY_OUTPUT=$(curl -sf -N --max-time 10 "${BASE_URL}/api/runs/${RUN_ID}/stream" 2>&1 || true)
REPLAY_HAS_CHUNK=$(echo "$REPLAY_OUTPUT" | grep -c '"chunk"' 2>/dev/null)
REPLAY_HAS_DONE=$(echo "$REPLAY_OUTPUT" | grep -c "^event: done" 2>/dev/null)

if [ "$REPLAY_HAS_CHUNK" -gt 0 ] && [ "$REPLAY_HAS_DONE" -gt 0 ]; then
    pass "Completed run replay returns stored output + done event"
else
    fail "Completed run replay missing chunk or done event (chunk=${REPLAY_HAS_CHUNK}, done=${REPLAY_HAS_DONE})"
fi

# ── Cleanup (operation) ───────────────────────────────────────────────────────

echo "Cleanup: Deleting test job..."
curl -sf -X DELETE "${BASE_URL}/api/jobs/${JOB_ID}" >/dev/null 2>&1 || true

# ── Summary ───────────────────────────────────────────────────────────────────

echo ""
echo "RESULTS: ${PASS} passed, ${FAIL} failed"
if [ "$FAIL" -gt 0 ]; then
    exit 1
fi
