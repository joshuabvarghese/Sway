# Sway demo — local OpenSearch cluster + live dashboard

Run Sway against a real (local) OpenSearch cluster and watch it rebalance
shards live in OpenSearch Dashboards.

## Prerequisites
- Docker + Docker Compose
- Go 1.21+ (`brew install go`) — build Sway for your own machine; the
  binary that may ship in the project root could be built for a different
  OS/architecture.

## 0. Build Sway (from the project root, one level up)
```bash
cd ..
go build -o sway-mac ./cmd/rebalancer
cd demo
```

## 1. Start the cluster
```bash
docker-compose up -d
```
Wait ~30-60s for the 3 nodes to elect a cluster manager.

## 2. Seed baseline indices
```bash
chmod +x seed-cluster.sh create-skew.sh monitor-cluster.sh
./seed-cluster.sh
```

## 3. Start the metrics monitor (leave running in its own terminal tab)
```bash
./monitor-cluster.sh
```
Writes a snapshot of per-node shard count and disk usage into a
`sway-metrics` index every 5 seconds — the data feed for your dashboard.

## 4. Build the dashboard
Open http://localhost:5601

- ☰ → Dashboards Management → Index Patterns → Create index pattern
  - Pattern: `sway-metrics`, time field: `@timestamp`
- ☰ → Visualize → Create visualization → Line
  - Index pattern: `sway-metrics`
  - Y-axis: Average of `shards` (add a second viz for `disk_percent` if you want it)
  - X-axis: Date Histogram on `@timestamp`
  - Split series: Terms on `node`
  - Save it
- ☰ → Dashboard → Create dashboard → Add your saved visualization(s)
  - Set time range to "Last 15 minutes", turn on auto-refresh (5s)

You now have a live, per-node line chart updating every 5 seconds.

## 5. Create real skew
```bash
./create-skew.sh
```
Pins a heavy index to `opensearch-node1` only, so it visibly spikes on
your dashboard relative to node2/node3.

## 6. Run Sway
```bash
# dry run first — plans moves, logs them, touches nothing
../sway-mac --config ../config.json --dry-run --once
```
If you're happy with the planned moves and want to see them actually
execute (and the dashboard flatten back out):
```bash
# edit ../config.json -> set "dry_run": false
../sway-mac --config ../config.json --once
```
Refresh the dashboard (or wait for auto-refresh) and watch node1's line
drop while node2/node3 climb.

## Cleanup
```bash
docker-compose down -v
```
