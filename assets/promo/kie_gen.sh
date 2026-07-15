#!/usr/bin/env bash
# Генератор карточек через KIE.ai (gpt4o-image). Синий SaaS-стиль VexaVPN.
# Использование: KIE_KEY=xxx ./kie_gen.sh "<prompt>" <out.png> [size]
#   size: 2:3 (портрет, по умолчанию) | 1:1 | 3:2
set -euo pipefail

KEY="${KIE_KEY:?set KIE_KEY}"
PROMPT="${1:?prompt required}"
OUT="${2:?out path required}"
SIZE="${3:-2:3}"
BASE="https://api.kie.ai/api/v1/gpt4o-image"

echo ">> createTask ($SIZE)"
RESP=$(curl -s -m 60 -X POST "$BASE/generate" \
  -H "Authorization: Bearer $KEY" -H "Content-Type: application/json" \
  -d "$(jq -n --arg p "$PROMPT" --arg s "$SIZE" \
        '{prompt:$p, size:$s, nVariants:1, isEnhance:false, enableFallback:true}')")
echo "$RESP" | head -c 400; echo
TASK=$(echo "$RESP" | jq -r '.data.taskId // .data.task_id // empty')
[ -z "$TASK" ] && { echo "!! no taskId"; exit 1; }
echo ">> taskId=$TASK ; polling..."

for i in $(seq 1 40); do
  sleep 6
  R=$(curl -s -m 30 "$BASE/record-info?taskId=$TASK" -H "Authorization: Bearer $KEY")
  ST=$(echo "$R" | jq -r '.data.status // .data.successFlag // empty')
  URL=$(echo "$R" | jq -r '.data.response.resultUrls[0] // .data.resultUrls[0] // (.data.response.result_urls[0]) // empty' 2>/dev/null)
  echo "  [$i] status=$ST"
  if [ -n "$URL" ] && [ "$URL" != "null" ]; then
    echo ">> done: $URL"
    curl -s -m 120 -o "$OUT" "$URL"
    echo ">> saved: $OUT ($(stat -c %s "$OUT") bytes)"
    exit 0
  fi
  case "$ST" in
    GENERATE_FAILED|FAILED|CREATE_TASK_FAILED) echo "!! failed: $R"; exit 1;;
  esac
done
echo "!! timeout"; exit 1
