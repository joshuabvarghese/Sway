#!/bin/bash
# Forces real imbalance: dumps a lot of documents into a single index with
# zero replicas and only 1 shard, then forcibly allocates it to one node.
# This gives Sway (and your dashboard) a visible disk/shard skew to fix.

HOST="http://localhost:9200"

echo "Creating a heavy index pinned to opensearch-node1..."
curl -s -X PUT "$HOST/heavy-2024" -H 'Content-Type: application/json' -d '{
  "settings": {
    "number_of_shards": 1,
    "number_of_replicas": 0,
    "index.routing.allocation.require._name": "opensearch-node1"
  }
}' > /dev/null

echo "Bulk loading documents (this takes a minute)..."
for batch in $(seq 1 20); do
  PAYLOAD=""
  for i in $(seq 1 500); do
    PAYLOAD="$PAYLOAD{\"index\":{\"_index\":\"heavy-2024\"}}
{\"msg\":\"padding document with extra text to take up more disk space on purpose $i $batch\",\"level\":\"info\",\"batch\":$batch}
"
  done
  curl -s -X POST "$HOST/_bulk" -H 'Content-Type: application/x-ndjson' -d "$PAYLOAD" > /dev/null
  echo "  batch $batch/20 done"
done

echo ""
echo "Skew created. Check it with:"
echo "  curl -s '$HOST/_cat/allocation?v'"
echo ""
echo "node1 should now show noticeably more shards/disk than node2 and node3."
echo "Now run Sway:"
echo "  ./sway-mac --config config.json --dry-run --once"
echo "and watch your sway-metrics dashboard for the spike, then the rebalance."
