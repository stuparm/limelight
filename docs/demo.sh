#!/bin/bash
# Regenerates the README's terminal images.
#
#   go install github.com/charmbracelet/freeze@latest
#   docs/demo.sh
#
# demo-logs.raw.txt and demo-enable.txt are real output from examples/go-log,
# captured with a 10s TTL so the switch closes itself inside the window.
#
# Two presentation edits, both applied below so they are reproducible rather
# than hand-made:
#   - the date is stripped from both timestamp formats, keeping hh:mm:ss. Over a
#     20-second window the date carries no information and costs 24 columns.
#   - the hostname in the enable response was replaced with a placeholder at
#     capture time, since it is a personal machine name.
# Re-capture rather than editing by hand: an image nobody can reproduce is
# worse than no image.
set -euo pipefail
cd "$(dirname "$0")/.."
FREEZE="${FREEZE:-$(go env GOPATH)/bin/freeze}"

awk 'NR==1 && $0 ~ /^[[:space:]]*$/ {next} {print}' docs/demo-logs.raw.txt \
  | sed -E 's|^[0-9]{4}/[0-9]{2}/[0-9]{2} ([0-9]{2}:[0-9]{2}:[0-9]{2})|\1|' \
  | sed -E 's|"time":"[0-9]{4}-[0-9]{2}-[0-9]{2}T([0-9]{2}:[0-9]{2}:[0-9]{2})[0-9.+:]*"|"time":"\1"|' \
  > docs/demo-logs.txt

common=(--window --padding 20 --margin 12 --font.size 13 --line-height 1.3)
"$FREEZE" docs/demo-logs.txt   -o docs/demo-logs.svg   --wrap 152 "${common[@]}"
"$FREEZE" docs/demo-enable.txt -o docs/demo-enable.svg --wrap 84  "${common[@]}"
