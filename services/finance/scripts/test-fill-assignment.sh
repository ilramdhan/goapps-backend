#!/bin/bash
# Manual test script for fill-assignment CRUD via HTTP gateway.
# Usage:
#   BASE_URL=http://localhost:8080 JWT_TOKEN=<token> bash scripts/test-fill-assignment.sh
#
# Requires: curl, jq

set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
TOKEN="${JWT_TOKEN:-}"

AUTH_HEADER=()
if [[ -n "$TOKEN" ]]; then
    AUTH_HEADER=(-H "Authorization: Bearer $TOKEN")
fi

echo "=== Fill Assignment Config CRUD Test ==="
echo "Base URL: $BASE_URL"
echo ""

# ---------------------------------------------------------------------------
echo "--- 1. List global level configs ---"
curl -sf "${AUTH_HEADER[@]}" \
    "$BASE_URL/api/v1/finance/fill-configs/global" | jq .
echo ""

# ---------------------------------------------------------------------------
echo "--- 2. Upsert level 1 config (DEPT/COSTING filler, 48h SLA) ---"
curl -sf -X PUT "${AUTH_HEADER[@]}" \
    -H "Content-Type: application/json" \
    -d '{
      "route_level": 1,
      "filler_type": "DEPT",
      "filler_value": "COSTING",
      "approver_type": "DEPT",
      "approver_value": "COSTING_HEAD",
      "reapprove_on_change": false,
      "sla_fill_hours": 48,
      "sla_approve_hours": 24
    }' \
    "$BASE_URL/api/v1/finance/fill-configs" | jq .
echo ""

# ---------------------------------------------------------------------------
echo "--- 3. List global configs after upsert ---"
curl -sf "${AUTH_HEADER[@]}" \
    "$BASE_URL/api/v1/finance/fill-configs/global" | jq .
echo ""

# ---------------------------------------------------------------------------
echo "--- 4. List fill tasks (all statuses) ---"
curl -sf "${AUTH_HEADER[@]}" \
    "$BASE_URL/api/v1/finance/fill-tasks" | jq .
echo ""

# ---------------------------------------------------------------------------
echo "--- 5. List ACTIVE fill tasks ---"
curl -sf "${AUTH_HEADER[@]}" \
    "$BASE_URL/api/v1/finance/fill-tasks?status=ACTIVE" | jq .
echo ""

# ---------------------------------------------------------------------------
echo "--- 6. List FILLING fill tasks ---"
curl -sf "${AUTH_HEADER[@]}" \
    "$BASE_URL/api/v1/finance/fill-tasks?status=FILLING" | jq .
echo ""

# ---------------------------------------------------------------------------
echo "--- 7. List FILLED fill tasks ---"
curl -sf "${AUTH_HEADER[@]}" \
    "$BASE_URL/api/v1/finance/fill-tasks?status=FILLED" | jq .
echo ""

# ---------------------------------------------------------------------------
echo "--- 8. Deactivate level 3 global config ---"
curl -sf -X DELETE "${AUTH_HEADER[@]}" \
    "$BASE_URL/api/v1/finance/fill-configs/global/3" | jq .
echo ""

# ---------------------------------------------------------------------------
echo "--- 9. List global configs after deactivate (should show 2 active) ---"
curl -sf "${AUTH_HEADER[@]}" \
    "$BASE_URL/api/v1/finance/fill-configs/global" | jq .
echo ""

echo "=== Done ==="
