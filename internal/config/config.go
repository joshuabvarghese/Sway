// Package config provides cloud-agnostic configuration for Project Sway.
// Connection details are endpoint-only: no cloud-provider SDK types appear here.
// To target AWS, GCP, or On-Premise, simply change the Addresses field.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// Config is the root configuration object.
type Config struct {
	OpenSearch     OpenSearchConfig     `json:"opensearch"`
	Agent          AgentConfig          `json:"agent"`
	Rebalancer     RebalancerConfig     `json:"rebalancer"`
	CircuitBreaker CircuitBreakerConfig `json:"circuit_breaker"`
}

// OpenSearchConfig holds connection details.
// Cloud-agnostic: the engine speaks only to the OpenSearch REST API.
// For cloud-specific auth (AWS SigV4, GCP OIDC), plug a custom
// http.RoundTripper into the HTTPClient — no code changes elsewhere.
type OpenSearchConfig struct {
	// Addresses: any reachable OpenSearch endpoint.
	//   AWS:        ["https://search-my-domain.us-east-1.es.amazonaws.com"]
	//   GCP:        ["https://opensearch.example.internal:9200"]
	//   On-Premise: ["http://10.0.0.1:9200","http://10.0.0.2:9200"]
	Addresses      []string `json:"addresses"`
	Username       string   `json:"username"`
	Password       string   `json:"password"`
	TLSVerify      bool     `json:"tls_verify"`
	TimeoutSeconds int      `json:"timeout_seconds"`
}

// AgentConfig controls the monitoring agent.
type AgentConfig struct {
	// PollIntervalSeconds defines how often metrics are scraped.
	PollIntervalSeconds int `json:"poll_interval_seconds"`
	// HotNodeThreshold: weighted score above which a node is classified HOT (0.0–1.0).
	HotNodeThreshold float64 `json:"hot_node_threshold"`
	// Metric weights. Must conceptually sum to 1.0.
	JVMWeight   float64 `json:"jvm_weight"`
	DiskWeight  float64 `json:"disk_weight"`
	ShardWeight float64 `json:"shard_weight"`
}

// PollInterval returns poll interval as a time.Duration.
func (a *AgentConfig) PollInterval() time.Duration {
	return time.Duration(a.PollIntervalSeconds) * time.Second
}

// RebalancerConfig controls rebalancing behaviour.
type RebalancerConfig struct {
	// DryRun: plan moves but make no API calls. Defaults to true (safety first).
	DryRun bool `json:"dry_run"`
	// MaxMovesPerCycle caps the number of shard relocations per pass.
	MaxMovesPerCycle int `json:"max_moves_per_cycle"`
	// SkewReductionTarget: fraction of current skew to eliminate per cycle.
	// 0.25 means "reduce skew by 25% per cycle."
	SkewReductionTarget float64 `json:"skew_reduction_target"`
	// LargeShardThresholdBytes: shards above this size are "large" and
	// prioritised for early movement (moves the most data with fewest API calls).
	LargeShardThresholdBytes int64 `json:"large_shard_threshold_bytes"`
}

// CircuitBreakerConfig defines the safety thresholds.
type CircuitBreakerConfig struct {
	// RequiredHealth: minimum cluster health before automation runs.
	// "green" is recommended for production; "yellow" is permitted for testing.
	RequiredHealth string `json:"required_health"`
	// MaxAvgLatencyMs: if average search latency exceeds this value (ms),
	// the circuit opens. Proxy for p99; integrate an APM system for true p99.
	MaxAvgLatencyMs float64 `json:"max_avg_latency_ms"`
	// MaxRelocatingShards: block automation if shards are already moving.
	// Set to 0 to enforce strict serial execution.
	MaxRelocatingShards int `json:"max_relocating_shards"`
}

// Default returns a production-safe configuration with conservative settings.
// DryRun is true and thresholds are strict by default.
func Default() *Config {
	return &Config{
		OpenSearch: OpenSearchConfig{
			Addresses:      []string{"http://localhost:9200"},
			TLSVerify:      true,
			TimeoutSeconds: 30,
		},
		Agent: AgentConfig{
			PollIntervalSeconds: 30,
			HotNodeThreshold:    0.70,
			JVMWeight:           0.40,
			DiskWeight:          0.40,
			ShardWeight:         0.20,
		},
		Rebalancer: RebalancerConfig{
			DryRun:                   true,
			MaxMovesPerCycle:         5,
			SkewReductionTarget:      0.25,
			LargeShardThresholdBytes: 2 * 1024 * 1024 * 1024, // 2 GiB
		},
		CircuitBreaker: CircuitBreakerConfig{
			RequiredHealth:      "green",
			MaxAvgLatencyMs:     200.0,
			MaxRelocatingShards: 0,
		},
	}
}

// LoadFromFile reads a JSON config file and overlays it on top of defaults.
func LoadFromFile(path string) (*Config, error) {
	cfg := Default()
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening config %q: %w", path, err)
	}
	defer f.Close()
	if err := json.NewDecoder(f).Decode(cfg); err != nil {
		return nil, fmt.Errorf("decoding config %q: %w", path, err)
	}
	return cfg, nil
}
