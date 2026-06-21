package opensearch

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// ──────────────────────────────────────────────────────────────
//  Client interface — the cloud-agnostic abstraction boundary
// ──────────────────────────────────────────────────────────────

// Client is the single interface that every backend must satisfy.
// The HTTP implementation targets vanilla OpenSearch (on-premise or any cloud).
// The SimulatedClient (internal/simulation) satisfies the same interface.
//
// Cloud-specific auth (AWS SigV4, GCP workload identity) is handled by
// supplying a custom http.RoundTripper to NewHTTPClient — no interface changes.
type Client interface {
	GetNodesStats(ctx context.Context) (*NodesStatsResponse, error)
	GetClusterState(ctx context.Context) (*ClusterStateResponse, error)
	GetClusterHealth(ctx context.Context) (*ClusterHealthResponse, error)
	GetShardSizes(ctx context.Context) (map[string]int64, error) // key: "index/shard/p|r"
	Reroute(ctx context.Context, req *RerouteRequest, dryRun bool) (*RerouteResponse, error)
}

// ──────────────────────────────────────────────────────────────
//  HTTP implementation
// ──────────────────────────────────────────────────────────────

// HTTPClient talks directly to the OpenSearch REST API.
// It is stateless and safe for concurrent use.
type HTTPClient struct {
	baseURL    string
	username   string
	password   string
	httpClient *http.Client
}

// HTTPClientOption is a functional option for NewHTTPClient.
type HTTPClientOption func(*HTTPClient)

// WithTransport replaces the underlying http.RoundTripper.
// Use this to inject AWS SigV4 signing, GCP token refresh, or mTLS.
func WithTransport(rt http.RoundTripper) HTTPClientOption {
	return func(c *HTTPClient) {
		c.httpClient.Transport = rt
	}
}

// NewHTTPClient constructs an HTTPClient from connection parameters.
func NewHTTPClient(address, username, password string, tlsVerify bool, timeout time.Duration, opts ...HTTPClientOption) *HTTPClient {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: !tlsVerify}, //nolint:gosec
	}
	c := &HTTPClient{
		baseURL:  address,
		username: username,
		password: password,
		httpClient: &http.Client{
			Timeout:   timeout,
			Transport: tr,
		},
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// get performs a GET request and decodes JSON into v.
func (c *HTTPClient) get(ctx context.Context, path string, v interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("building request for %s: %w", path, err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.username != "" {
		req.SetBasicAuth(c.username, c.password)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GET %s returned %d: %s", path, resp.StatusCode, body)
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

// post performs a POST request with a JSON body and decodes the response.
func (c *HTTPClient) post(ctx context.Context, path string, body interface{}, v interface{}) error {
	b, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshalling body: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("building request for %s: %w", path, err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.username != "" {
		req.SetBasicAuth(c.username, c.password)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("POST %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body2, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("POST %s returned %d: %s", path, resp.StatusCode, body2)
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

// GetNodesStats calls GET /_nodes/stats.
func (c *HTTPClient) GetNodesStats(ctx context.Context) (*NodesStatsResponse, error) {
	var r NodesStatsResponse
	return &r, c.get(ctx, "/_nodes/stats", &r)
}

// GetClusterState calls GET /_cluster/state/routing_table,nodes.
func (c *HTTPClient) GetClusterState(ctx context.Context) (*ClusterStateResponse, error) {
	var r ClusterStateResponse
	return &r, c.get(ctx, "/_cluster/state/routing_table,nodes", &r)
}

// GetClusterHealth calls GET /_cluster/health.
func (c *HTTPClient) GetClusterHealth(ctx context.Context) (*ClusterHealthResponse, error) {
	var r ClusterHealthResponse
	return &r, c.get(ctx, "/_cluster/health", &r)
}

// GetShardSizes calls GET /_cat/shards?format=json&bytes=b and returns a map
// keyed by "index/shardNum/p|r" → size in bytes.
func (c *HTTPClient) GetShardSizes(ctx context.Context) (map[string]int64, error) {
	var rows []CatShard
	if err := c.get(ctx, "/_cat/shards?format=json&bytes=b&h=index,shard,prirep,store,node,state", &rows); err != nil {
		return nil, err
	}
	out := make(map[string]int64, len(rows))
	for _, row := range rows {
		if row.State != "STARTED" {
			continue
		}
		size, _ := strconv.ParseInt(row.Store, 10, 64)
		key := fmt.Sprintf("%s/%s/%s", row.Index, row.Shard, row.Prirep)
		out[key] = size
	}
	return out, nil
}

// Reroute calls POST /_cluster/reroute.
// When dryRun is true the call is skipped and a synthetic acknowledged response
// is returned — the engine still logs the full payload.
func (c *HTTPClient) Reroute(ctx context.Context, req *RerouteRequest, dryRun bool) (*RerouteResponse, error) {
	if dryRun {
		return &RerouteResponse{Acknowledged: true}, nil
	}
	var r RerouteResponse
	return &r, c.post(ctx, "/_cluster/reroute", req, &r)
}
