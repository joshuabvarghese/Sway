#!/bin/bash
HOST="http://localhost:9200"
SWAY="../sway-mac"
CONFIG="../config.json"
CYCLE_PAUSE=20

if [ ! -f "$SWAY" ]; then
  echo "Error: $SWAY not found. Build it first from the project root:"
  echo "  go build -o sway-mac ./cmd/rebalancer"
  exit 1
fi

echo "=== Sway live demo loop ==="
echo "Open http://localhost:5601 and watch your dashboard now."
echo "Press Ctrl+C anytime to stop."
echo ""

cycle=1
while true; do
  echo "--- Cycle $cycle: creating skew ---"
  curl -s -X PUT "$HOST/heavy-2024/_settings" -H 'Content-Type: application/json' -d '{
    "index.routing.allocation.require._name": "opensearch-node1"
  }' > /dev/null

  echo "Waiting ${CYCLE_PAUSE}s for the spike to show on the dashboard..."
  sleep $CYCLE_PAUSE

  echo "--- Cycle $cycle: clearing pin and running Sway live ---"
  curl -s -X PUT "$HOST/heavy-2024/_settings" -H 'Content-Type: application/json' -d '{
    "index.routing.allocation.require._name": null
  }' > /dev/null

  "$SWAY" --config "$CONFIG" --once

  echo "Waiting ${CYCLE_PAUSE}s for the rebalance to show on the dashboard..."
  sleep $CYCLE_PAUSE

  cycle=$((cycle + 1))
  echo ""
done
