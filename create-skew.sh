#!/bin/bash
HOST="http://localhost:9200"

echo "Fetching current shard layout..."
curl -s "$HOST/_cat/shards?format=json" > /tmp/sway-shards.json

python3 << PYEOF
import json, subprocess

with open('/tmp/sway-shards.json') as f:
    shards = json.load(f)

moves = [s for s in shards if s.get('node') and s.get('node') != 'opensearch-node1' and s.get('prirep') == 'p']
moves = moves[:10]

if not moves:
    print('No eligible shards found to move.')
else:
    commands = [{
        'move': {
            'index': s['index'],
            'shard': int(s['shard']),
            'from_node': s['node'],
            'to_node': 'opensearch-node1'
        }
    } for s in moves]

    body = json.dumps({'commands': commands})
    print(f'Rerouting {len(commands)} shard(s) onto opensearch-node1...')

    result = subprocess.run(
        ['curl', '-s', '-X', 'POST', 'http://localhost:9200/_cluster/reroute',
         '-H', 'Content-Type: application/json', '-d', body],
        capture_output=True, text=True
    )
    print(result.stdout[:800])
PYEOF

echo ""
echo "Waiting for relocation to finish..."
sleep 8
curl -s "$HOST/_cat/allocation?v"
echo ""
echo "Now re-run Sway:"
echo "  ../sway-mac --config ../config.json --dry-run --once"
