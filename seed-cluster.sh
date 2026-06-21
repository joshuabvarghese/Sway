#!/bin/bash
# Creates a deliberately skewed set of indices on the 3-node cluster
# so Sway has real rebalancing work to do.
# Run this AFTER `docker-compose up -d` and waiting ~30s for the cluster to go green.

HOST="http://localhost:9200"

echo "Waiting for cluster health..."
until curl -s "$HOST/_cluster/health" | grep -q '"status":"green"'; do
  sleep 2
  echo "  ...still waiting"
done
echo "Cluster is green."

echo "Creating indices..."

curl -s -X PUT "$HOST/logs-2024" -H 'Content-Type: application/json' -d '{
  "settings": { "number_of_shards": 6, "number_of_replicas": 1 }
}' > /dev/null

curl -s -X PUT "$HOST/traces-2024" -H 'Content-Type: application/json' -d '{
  "settings": { "number_of_shards": 4, "number_of_replicas": 1 }
}' > /dev/null

curl -s -X PUT "$HOST/metrics-2024" -H 'Content-Type: application/json' -d '{
  "settings": { "number_of_shards": 3, "number_of_replicas": 1 }
}' > /dev/null

echo "Indexing some sample documents to give the indices real size..."
for i in $(seq 1 2000); do
  curl -s -X POST "$HOST/logs-2024/_doc" -H 'Content-Type: application/json' \
    -d "{\"msg\":\"sample log line $i\",\"level\":\"info\"}" > /dev/null
done

echo "Done. Check distribution with:"
echo "  curl -s '$HOST/_cat/shards?v'"
echo "  curl -s '$HOST/_cat/allocation?v'"
echo "Or open http://localhost:5601 -> Dev Tools / Index Management to see it visually."
