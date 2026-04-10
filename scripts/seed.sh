#!/usr/bin/env bash
set -euo pipefail
BASE_URL=${BASE_URL:-http://localhost:8080}

curl -sS -X POST "$BASE_URL/monitors" -H 'Content-Type: application/json' -d '{"name":"Example","url":"https://example.com","interval_seconds":30,"timeout_seconds":5,"is_enabled":true}'
echo
curl -sS -X POST "$BASE_URL/monitors" -H 'Content-Type: application/json' -d '{"name":"HTTPBin","url":"https://httpbin.org/status/200","interval_seconds":45,"timeout_seconds":5,"is_enabled":true}'
echo
