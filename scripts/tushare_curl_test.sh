#!/usr/bin/env bash

set -euo pipefail

API_URL="http://127.0.0.1:1155/dataapi"
TOKEN="${TUSHARE_TOKEN:-}"
EXPIRES_AT_TS="$(( $(date +%s) + 1800 ))"

if [[ -z "$TOKEN" ]]; then
  echo "Error: TUSHARE_TOKEN is not set."
  echo "Usage example:"
  echo "  TUSHARE_TOKEN=your_token ./scripts/tushare_curl_test.sh all"
  echo "  TUSHARE_TOKEN=your_token ./scripts/tushare_curl_test.sh ttl"
  exit 1
fi

print_json() {
  printf '%s\n' "$1"
}

usage() {
  cat <<EOF
Usage:
  TUSHARE_TOKEN=your_token ./scripts/tushare_curl_test.sh <scenario> [scenario...]

Available scenarios:
  all
  ttl
  no-cache
  expires-at
EOF
}

run_request() {
  local label="$1"
  local payload="$2"
  local response

  response="$(
    curl -sS \
      -X POST "$API_URL" \
      -H "Content-Type: application/json" \
      --data-raw "$payload"
  )"

  echo
  echo "=== $label ==="
  echo "POST $API_URL"
  echo "Request payload:"
  print_json "$payload"
  echo "Response body:"
  print_json "$response"
}

build_ttl_payload() {
  cat <<EOF
{
  "api_name": "stock_basic",
  "token": "$TOKEN",
  "params": {
    "list_status": "L", "limit": 1
  },
  "fields": "ts_code,symbol,name,area,industry,list_date",
  "_cache": {
    "namespace": "script.stock_basic.ttl",
    "ttl": 300,
    "no_cache": false
  }
}
EOF
}

build_no_cache_payload() {
  cat <<EOF
{
  "api_name": "stock_basic",
  "token": "$TOKEN",
  "params": {
    "list_status": "L", "limit": 1
  },
  "fields": "ts_code,symbol,name,area,industry,list_date",
  "_cache": {
    "namespace": "script.stock_basic.ttl",
    "no_cache": true
  }
}
EOF
}

build_expires_at_payload() {
  cat <<EOF
{
  "api_name": "stock_basic",
  "token": "$TOKEN",
  "params": {
    "list_status": "L", "limit": 1
  },
  "fields": "ts_code,symbol,name,area,industry,list_date",
  "_cache": {
    "namespace": "script.stock_basic.expires_at",
    "expires_at": $EXPIRES_AT_TS,
    "no_cache": false
  }
}
EOF
}

run_scenario() {
  local scenario="$1"

  case "$scenario" in
    ttl)
      run_request "ttl request" "$(build_ttl_payload)"
      ;;
    no-cache)
      run_request "no_cache request" "$(build_no_cache_payload)"
      ;;
    expires-at)
      run_request "expires_at request" "$(build_expires_at_payload)"
      ;;
    *)
      echo "Error: unknown scenario '$scenario'"
      usage
      exit 1
      ;;
  esac
}

if [[ $# -eq 0 ]]; then
  usage
  exit 1
fi

echo "Running myproxy cache scenario test against $API_URL"
echo "Use myproxy logs to observe cache_status changes."
echo "expires_at timestamp: $EXPIRES_AT_TS"

for scenario in "$@"; do
  if [[ "$scenario" == "all" ]]; then
    run_scenario "ttl"
    run_scenario "no-cache"
    run_scenario "expires-at"
  else
    run_scenario "$scenario"
  fi
done
