package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ============================================================
// Decentralized Relay Handler
// ============================================================
//
// Route: ANY /network/{node_id}/{rest...}
//
// When a shared-network node receives a request at /network/{node_id}/...,
// it acts as a relay:
//   1. Look up node_id in the route table
//   2. If found → reverse-proxy the request to the target node
//   3. If not found → try querying bootstrap nodes (Phase 1: return 404)
//   4. Hop-count header prevents infinite loops (max 3)
//
// The target node receives the request with /network/{node_id} stripped,
// so /network/mmx-abc123/v1/chat/completions → target sees /v1/chat/completions
// This ensures OpenAI SDK compatibility at the target.

const (
	headerRelayHop  = "X-ModelMux-Hop"
	headerRelayFrom = "X-ModelMux-Relay-From"
)

// handleNetworkRelay handles relay requests: /network/{node_id}/{rest...}
func handleNetworkRelay(w http.ResponseWriter, r *http.Request) {
	// Only serve in shared mode
	if netMgr == nil || !netMgr.IsSharedMode() {
		writeError(w, 404, "shared network not active")
		return
	}

	// Extract node_id from path: /network/{node_id}/...
	path := strings.TrimPrefix(r.URL.Path, "/network/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		writeError(w, 400, "missing node_id in path")
		return
	}
	targetNodeID := parts[0]

	// Validate NodeID format
	if !strings.HasPrefix(targetNodeID, p2pNodeIDPrefix) {
		writeError(w, 400, "invalid node_id format")
		return
	}

	// Check hop count to prevent loops
	hopCount := 0
	if hopStr := r.Header.Get(headerRelayHop); hopStr != "" {
		hopCount, _ = strconv.Atoi(hopStr)
	}
	if hopCount >= maxRelayHops {
		writeError(w, 508, "max relay hops exceeded")
		slog.Warn("relay loop detected", "node_id", targetNodeID, "hops", hopCount)
		return
	}

	// If the target is ourselves, handle locally
	selfID := netMgr.GetNodeID()
	if targetNodeID == selfID {
		handleRelayToLocal(w, r, parts, hopCount)
		return
	}

	// Resolve target node in route table
	entry := routeTable.Get(targetNodeID)
	if entry == nil {
		// Phase 1: query bootstrap nodes (simplified)
		// Phase 2: full DHT lookup via libp2p
		entry = queryBootstrapForNode(targetNodeID)
	}

	if entry == nil || len(entry.Addresses) == 0 {
		writeJSON(w, 404, map[string]any{
			"error":   "node not found",
			"node_id": targetNodeID,
			"message": "target node not found in route table. It may be offline or not yet registered.",
		})
		return
	}

	// Forward request via reverse proxy to the target node
	relayToRemote(w, r, entry, parts, hopCount)
}

// handleRelayToLocal handles requests targeting this node itself
// Strips /network/{node_id} prefix and serves the remaining path locally
func handleRelayToLocal(w http.ResponseWriter, r *http.Request, parts []string, hopCount int) {
	netMgr.RecordReceived()

	// Reconstruct path without the /network/{node_id} prefix
	restPath := ""
	if len(parts) > 1 {
		restPath = "/" + parts[1]
	} else {
		restPath = "/"
	}

	// Rewrite the request path
	r.URL.Path = restPath
	r.RequestURI = restPath
	if r.URL.RawQuery != "" {
		r.RequestURI += "?" + r.URL.RawQuery
	}

	slog.Info("relay to local", "target", "self", "path", restPath, "hops", hopCount)

	// Serve the rewritten request using the main handler
	// We re-dispatch to the main mux by calling the server's handler
	// The simplest way: construct a new request and serve it
	localPort := cfg.Get("service_port", "8000")
	target, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%s", localPort))

	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = target.Scheme
			req.URL.Host = target.Host
			req.URL.Path = restPath
			req.URL.RawQuery = r.URL.RawQuery
			req.Host = target.Host
			// Remove relay headers for local delivery
			req.Header.Del(headerRelayHop)
			req.Header.Del(headerRelayFrom)
		},
		ErrorHandler: func(w2 http.ResponseWriter, r2 *http.Request, err error) {
			slog.Error("local relay proxy error", "error", err)
			writeError(w2, 502, "local relay failed")
		},
	}

	proxy.ServeHTTP(w, r)
}

// relayToRemote forwards a request to a remote node via reverse proxy
func relayToRemote(w http.ResponseWriter, r *http.Request, entry *RouteEntry, parts []string, hopCount int) {
	// Pick the best address (prefer HTTPS)
	targetAddr := pickBestAddress(entry.Addresses)
	if targetAddr == "" {
		writeError(w, 502, "no reachable address for node")
		return
	}

	target, err := url.Parse(targetAddr)
	if err != nil {
		writeError(w, 502, "invalid target address")
		return
	}

	// Reconstruct the path: /network/{node_id}/{rest} → /network/{node_id}/{rest}
	// We keep the full path so the target can also strip it if it's also a relay
	// Actually, we strip it so the target sees the original path: /{rest}
	restPath := ""
	if len(parts) > 1 {
		restPath = "/" + parts[1]
	} else {
		restPath = "/"
	}

	relayFrom := netMgr.GetNodeID()

	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = target.Scheme
			req.URL.Host = target.Host
			req.URL.Path = restPath
			req.URL.RawQuery = r.URL.RawQuery
			req.Host = target.Host

			// Set relay headers
			req.Header.Set(headerRelayHop, strconv.Itoa(hopCount+1))
			req.Header.Set(headerRelayFrom, relayFrom)

			// Preserve original auth headers (consumer key transparent to relay)
			// Do NOT strip Authorization — target node validates it
		},
		Transport: &http.Transport{
			MaxIdleConns:        10,
			IdleConnTimeout:     30 * time.Second,
			DisableCompression:  true, // don't compress SSE streams
		},
		ErrorHandler: func(w2 http.ResponseWriter, r2 *http.Request, err error) {
			slog.Error("relay to remote failed", "target", entry.NodeID, "addr", targetAddr, "error", err)
			netMgr.RecordRelayResult(false)
			writeError(w2, 502, fmt.Sprintf("relay to %s failed: %v", entry.NodeID, err))
		},
		ModifyResponse: func(resp *http.Response) error {
			netMgr.RecordRelayResult(resp.StatusCode < 400)
			return nil
		},
	}

	slog.Info("relaying to remote", "target_node", entry.NodeID, "addr", targetAddr, "path", restPath, "hop", hopCount+1)
	proxy.ServeHTTP(w, r)
}

// pickBestAddress selects the best address from a list (prefer HTTPS public URLs)
func pickBestAddress(addresses []string) string {
	if len(addresses) == 0 {
		return ""
	}
	// Prefer custom domain > tunnel URL > localhost
	var tunnelURL, localAddr string
	for _, a := range addresses {
		if strings.HasPrefix(a, "https://") && !strings.Contains(a, "trycloudflare.com") {
			return a // custom domain — best
		}
		if strings.Contains(a, "trycloudflare.com") {
			tunnelURL = a
		}
		if strings.HasPrefix(a, "http://localhost") {
			localAddr = a
		}
	}
	if tunnelURL != "" {
		return tunnelURL
	}
	if localAddr != "" {
		return localAddr
	}
	return addresses[0]
}

// queryBootstrapForNode queries bootstrap nodes for a NodeID (Phase 1 simplified)
// In Phase 2 this will be replaced by full DHT lookup via libp2p
func queryBootstrapForNode(nodeID string) *RouteEntry {
	if netMgr == nil {
		return nil
	}
	netMgr.mu.RLock()
	bootstrapNodes := make([]string, len(netMgr.config.BootstrapNodes))
	copy(bootstrapNodes, netMgr.config.BootstrapNodes)
	netMgr.mu.RUnlock()

	client := &http.Client{Timeout: 5 * time.Second}

	for _, bootstrapURL := range bootstrapNodes {
		resolveURL := fmt.Sprintf("%s/api/network/resolve/%s", strings.TrimRight(bootstrapURL, "/"), nodeID)
		resp, err := client.Get(resolveURL)
		if err != nil {
			continue
		}
		if resp.StatusCode != 200 {
			resp.Body.Close()
			continue
		}

		var result struct {
			NodeID    string   `json:"node_id"`
			NodeName  string   `json:"node_name"`
			Addresses []string `json:"addresses"`
			Status    string   `json:"status"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			resp.Body.Close()
			continue
		}
		resp.Body.Close()

		if len(result.Addresses) > 0 {
			// Cache in local route table
			routeTable.Put(result.NodeID, result.NodeName, result.Addresses)
			return &RouteEntry{
				NodeID:    result.NodeID,
				NodeName:  result.NodeName,
				Addresses: result.Addresses,
				Status:    result.Status,
			}
		}
	}
	return nil
}
