# Sway

**Automated shard rebalancing engine for OpenSearch — cloud-agnostic, safety-first, incremental by design.**

Built as an Instaclustr portfolio project, Sway continuously monitors a cluster's resource distribution, scores each node using a weighted composite metric, plans the minimum set of shard movements needed to reduce load skew, and executes those moves — all behind a layered circuit breaker that gates every action.

---

## Why Sway exists

OpenSearch clusters drift. A few indices grow faster than others, a handful of nodes absorb most of the query load, and suddenly two nodes are running hot while the rest sit idle. The built-in balancer handles shard counts but knows nothing about JVM pressure or disk saturation.

Sway watches those signals — heap usage, disk fill rate, shard count — combines them into a single hotness score per node, and moves shards from the hottest nodes to the coolest ones, largest shards first. It does just enough work per cycle to hit a configurable reduction target, then waits. The result is a cluster that converges toward balance incrementally rather than thrashing.

---

## Safety model

> *An automation that breaks a cluster is infinitely worse than one that does nothing.*

Every design decision flows from that principle.

**Layer 1 — Default immutability.** `dry_run` is `true` in the factory config. The engine plans and logs every move it *would* make without touching the cluster. Live execution requires a deliberate opt-in. A misconfigured deployment cannot accidentally move shards.

**Layer 2 — Circuit breaker.** Before each cycle begins, three conditions are checked against the live cluster:

| Check | What it measures | Default threshold |
|---|---|---|
| Cluster health | `_cluster/health` status | Must be `green` |
| Avg search latency | Mean query time across data nodes | Must be `< 200 ms` |
| Relocating shards | Active migrations already in flight | Must be `0` |

If any single check fails, the circuit opens and the cycle is aborted. Every check is logged with both its measured value and its configured threshold, so operators have a complete audit trail of why automation was blocked. The circuit is re-evaluated fresh on the next cycle — there is no half-open state or recovery assumption.

**Layer 3 — Conservative move sizing.** `max_moves_per_cycle` caps relocations per pass (default: 5). `skew_reduction_target` stops planning once the projected improvement meets the configured fraction (default: 25%). The engine does not try to fix everything in one shot.

**Layer 4 — Placement guards.** A shard's primary and its replica are never placed on the same destination node. A destination is skipped if placing the shard would push its projected disk usage above 90%.

---

## Architecture

```
cmd/rebalancer/
└── main.go                  CLI wiring; selects real vs simulated client

internal/
├── config/
│   └── config.go            Cloud-agnostic config; conservative defaults
│
├── opensearch/
│   ├── types.go             Exact API response types (_nodes/stats, _cluster/state, etc.)
│   └── client.go            Client interface + HTTP implementation
│
├── agent/
│   ├── metrics.go           NodeMetrics, ShardInfo, ClusterSnapshot types
│   └── agent.go             MonitoringAgent: scrapes APIs, computes hot-scores
│
├── circuitbreaker/
│   └── breaker.go           Safety gate: health + latency + relocating checks
│
├── rebalancer/
│   ├── target.go            TargetStateGenerator: plans minimal shard moves
│   └── engine.go            Orchestrates the full collect → check → plan → execute cycle
│
└── simulation/
    └── simulator.go         Virtual 10-node cluster implementing opensearch.Client
```

### Data flow

```
                   ┌─────────────────────────────────────────────────────┐
                   │                   Engine (per cycle)                │
                   │                                                     │
  OpenSearch API   │  ┌──────────────┐     ClusterSnapshot              │
  (real or sim) ──►│  │  Monitoring  ├─────────────────────►            │
                   │  │    Agent     │                                   │
                   │  └──────────────┘  ┌───────────────────┐           │
                   │                    │  Circuit Breaker   │           │
                   │  ClusterSnapshot──►│  • Health check    │           │
                   │                    │  • Latency check   │           │
                   │                    │  • Reloc. check    │           │
                   │                    └────────┬──────────┘           │
                   │                             │ CLOSED                │
                   │                    ┌────────▼──────────┐           │
                   │                    │  Target State Gen. │           │
                   │                    │  • Hot node score  │           │
                   │                    │  • Large-first sort│           │
                   │                    │  • Capacity guard  │           │
                   │                    └────────┬──────────┘           │
                   │                             │ ShardMoves            │
                   │                    ┌────────▼──────────┐           │
                   │                    │  Executor          │──►  API  │
                   │                    │  _cluster/reroute  │  (or log)│
                   │                    └───────────────────┘           │
                   └─────────────────────────────────────────────────────┘
```

---

## Hot node scoring

A node's hotness is a weighted composite of three normalised metrics:

```
HotScore = JVMWeight × (heapUsed%) + DiskWeight × (diskUsed%) + ShardWeight × (shardCount / maxShards)
```

Default weights:

| Metric | Weight | Rationale |
|---|---|---|
| JVM heap | **0.40** | Heap pressure is the most direct signal of query and indexing load |
| Disk usage | **0.40** | Disk saturation causes the hardest failures — writes stop entirely |
| Shard count | **0.20** | A proxy for query fan-out and indexing thread contention |

A node is classified **HOT** when its score reaches `hot_node_threshold` (default: `0.70`). All weights and the threshold are configurable without any code changes.

---

## Cloud-agnostic design

The `opensearch.Client` interface is the only abstraction boundary. Everything above it — the monitoring agent, circuit breaker, target-state generator, and executor — is cloud-neutral and has no knowledge of where the cluster is running.

```go
type Client interface {
    GetNodesStats(ctx context.Context) (*NodesStatsResponse, error)
    GetClusterState(ctx context.Context) (*ClusterStateResponse, error)
    GetClusterHealth(ctx context.Context) (*ClusterHealthResponse, error)
    GetShardSizes(ctx context.Context) (map[string]int64, error)
    Reroute(ctx context.Context, req *RerouteRequest, dryRun bool) (*RerouteResponse, error)
}
```

Cloud-specific auth is injected via a custom `http.RoundTripper` — no changes to any algorithm or safety check are required:

```go
// AWS OpenSearch Service — SigV4 signing
client := opensearch.NewHTTPClient(
    "https://search-mycluster.us-east-1.es.amazonaws.com",
    "", "", true, 30*time.Second,
    opensearch.WithTransport(sigV4RoundTripper),
)

// GCP Managed OpenSearch — token refresh
client := opensearch.NewHTTPClient(
    "https://opensearch.internal.example.com:9200",
    "", "", true, 30*time.Second,
    opensearch.WithTransport(gcpTokenTransport),
)

// On-Premise — basic auth, no custom transport needed
client := opensearch.NewHTTPClient(
    "https://10.0.0.1:9200",
    "admin", "secret", true, 30*time.Second,
)
```

---

## Getting started

### Prerequisites

- Go 1.21+

### Build

```bash
git clone https://github.com/project-sway/sway
cd sway
go build ./cmd/rebalancer
```

### Simulation demo (no cluster required)

Runs a virtual 10-node cluster — intentionally skewed — through 3 rebalancing cycles:

```bash
./rebalancer --simulate --cycles 3
```

The simulator starts with two HOT nodes (node-0 at 85% JVM heap with 6 shards, node-1 at 79% with 6 shards) and a skew score around 0.15. You'll watch the largest shards get redistributed first, and the cluster converge toward ~0.05 over three cycles.

### Dry run against a real cluster

```bash
./rebalancer --config config.json --dry-run --once
```

This plans and logs every move the engine would make, touches nothing, and exits. A good first step before enabling live execution.

### Continuous live rebalancing

```bash
./rebalancer --config config.json
```

Press `Ctrl+C` for a graceful shutdown.

---

## Configuration

A complete `config.json` with all defaults shown:

```json
{
  "opensearch": {
    "addresses": ["http://localhost:9200"],
    "username": "",
    "password": "",
    "tls_verify": true,
    "timeout_seconds": 30
  },
  "agent": {
    "poll_interval_seconds": 30,
    "hot_node_threshold": 0.70,
    "jvm_weight": 0.40,
    "disk_weight": 0.40,
    "shard_weight": 0.20
  },
  "rebalancer": {
    "dry_run": true,
    "max_moves_per_cycle": 5,
    "skew_reduction_target": 0.25,
    "large_shard_threshold_bytes": 2147483648
  },
  "circuit_breaker": {
    "required_health": "green",
    "max_avg_latency_ms": 200.0,
    "max_relocating_shards": 0
  }
}
```

`dry_run` defaults to `true`. The engine will not move a single shard until you explicitly set it to `false`.

---

## CLI reference

```
Usage: rebalancer [flags]

  --config string    Path to JSON config file (default: "config.json")
  --simulate         Run against a virtual 10-node cluster (no real cluster needed)
  --dry-run          Plan moves but do not execute (overrides config file)
  --once             Execute a single cycle then exit
  --cycles int       Number of simulation cycles (default: 3, --simulate only)
```

--

## Extending the engine

### True p99 latency

The current agent computes average latency from cumulative OpenSearch counters, which is a reasonable proxy but not a histogram percentile. To use true p99, implement a collector that reads from Prometheus, OpenTelemetry, or your APM backend and sets `snap.AvgSearchLatMs` accordingly. The circuit breaker and rebalancer need no changes.

### Adding a metric to the hot score

1. Add a field to `agent.NodeMetrics`.
2. Populate it in `agent.MonitoringAgent.CollectSnapshot`.
3. Add a weight field to `config.AgentConfig`.
4. Include the term in both `MonitoringAgent.hotScore` and `rebalancer.projectedNode.hotScore`.

### Plugging in a different execution backend

Implement `opensearch.Client` — for example, to target a different search engine's reroute API, or to stub moves in integration tests — and pass it to `rebalancer.NewEngine`. Nothing else in the stack changes.

---

## Simulation output walkthrough

```
SIMULATED CLUSTER INITIAL STATE
  node-0  85% JVM  20% disk  6 shards   HOT (score 0.62)
  node-1  79% JVM  15% disk  6 shards   HOT (score 0.58)
  node-2  71% JVM  11% disk  5 shards
  node-3–9            2 shards each     all underutilised

CYCLE 1
  Circuit: CLOSED (all checks pass)
  Planned 4 moves (largest first):
    logs-2024[0]/primary    22 GiB   node-0 → node-9
    traces-2024[0]/primary  19 GiB   node-0 → node-8
    logs-2024[1]/primary    16 GiB   node-0 → node-7
    logs-2024[2]/primary    14 GiB   node-1 → node-6
  Skew: 0.1509 → 0.1024  (31% reduction, target 25% ✓)

CYCLE 2
  3 moves — skew 0.1024 → 0.0666

CYCLE 3
  2 moves — skew 0.0666 → 0.0483
```

By cycle 3, the cluster has moved from a two-hot-node imbalance to all nodes within a narrow score band — using 9 targeted API calls, each chosen to maximise disk-pressure relief per move.

---

## Licence

MIT
