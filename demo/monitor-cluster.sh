#!/bin/bash
# Polls node allocation + cluster health every 5s and writes a timestamped
# snapshot into the "sway-metrics" index, so OpenSearch Dashboards has
# real time-series data to chart (like a Grafana exporter would).
#
# Usage: ./monitor-cluster.sh
# Leave it running in a terminal tab while you run Sway in another tab.

HOST="http://localhost:9200"
INDEX="sway-metrics"

# Create the index with a sensible mapping (first run only)
curl -s -X PUT "$HOST/$INDEX" -H 'Content-Type: application/json' -d '{
  "mappings": {
    "properties": {
      "@timestamp": { "type": "date" },
      "node": { "type": "keyword" },
      "shards": { "type": "integer" },
      "disk_used_gb": { "type": "float" },
      "disk_percent": { "type": "integer" }
    }
  }
}' > /dev/null

echo "Writing snapshots to '$INDEX' every 5s. Ctrl+C to stop."

while true; do
  TS=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

  curl -s "$HOST/_cat/allocation?format=json" | \
  python3 -c "
import json, sys, urllib.request

rows = json.load(sys.stdin)
for r in rows:
    doc = {
        '@timestamp': '$TS',
        'node': r.get('node'),
        'shards': int(r.get('shards') or 0),
        'disk_used_gb': float((r.get('disk.used') or '0gb').replace('gb','').replace('kb','').replace('mb','') or 0),
        'disk_percent': int(r.get('disk.percent') or 0)
    }
    req = urllib.request.Request(
        '$HOST/$INDEX/_doc',
        data=json.dumps(doc).encode(),
        headers={'Content-Type': 'application/json'},
        method='POST'
    )
    urllib.request.urlopen(req)
"
  sleep 5
done
