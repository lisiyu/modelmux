package main

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// ============================================================
// Node Heartbeat & Discovery (Phase 2)
// ============================================================
//
// Every 60 seconds, this node sends heartbeats to all known peers.
// Peers respond with their own status + gossip (known peers list).
// After 3 missed heartbeats, a peer is marked offline.

const (
	heartbeatInterval = 60 * time.Second
	maxMissedHeartbeats = 3
)

// HeartbeatPayload is sent/received in heartbeat requests.
type HeartbeatPayload struct {
	NodeID    string   `json:"node_id"`
	NodeName  string   `json:"node_name"`
	Addresses []string `json:"addresses"`
	Models    []string `json:"models"`
	Uptime    int64    `json:"uptime"`
	Version   string   `json:"version,omitempty"`
	Timestamp int64    `json:"timestamp"`
}

// HeartbeatResponse is returned by the heartbeat endpoint.
type HeartbeatResponse struct {
	Status string             `json:"status"`
	Peers  []HeartbeatPeerInfo `json:"peers,omitempty"` // gossip: other known peers
}

// HeartbeatPeerInfo is a peer entry in the gossip response.
type HeartbeatPeerInfo struct {
	NodeID    string   `json:"node_id"`
	NodeName  string   `json:"node_name"`
	Addresses []string `json:"addresses"`
	Models    []string `json:"models"`
	Status    string   `json:"status"`
}

// peerHeartbeatState tracks heartbeat health per peer.
type peerHeartbeatState struct {
	missedCount  int
	lastHeartbeat time.Time
}

var heartbeatStates = make(map[string]*peerHeartbeatState)

// startHeartbeatLoop begins the periodic heartbeat sender.
func startHeartbeatLoop() {
	go func() {
		ticker := time.NewTicker(heartbeatInterval)
		defer ticker.Stop()
		slog.Info("heartbeat loop started", "interval", heartbeatInterval)

		for {
			<-ticker.C
			if netMgr == nil || !netMgr.IsSharedMode() {
				continue
			}
			sendHeartbeats()
			checkPeerHealth()
		}
	}()
}

// sendHeartbeats sends heartbeat to all known peers.
func sendHeartbeats() {
	netMgr.mu.RLock()
	peers := make([]PeerInfo, len(netMgr.config.Peers))
	copy(peers, netMgr.config.Peers)
	netMgr.mu.RUnlock()

	// Build heartbeat payload
	var uptime int64
	if !netMgr.startTime.IsZero() {
		uptime = int64(time.Since(netMgr.startTime).Seconds())
	}

	hb := HeartbeatPayload{
		NodeID:    netMgr.GetNodeID(),
		NodeName:  netMgr.config.NodeName,
		Addresses: netMgr.collectAddresses(),
		Models:    netMgr.config.SharedModels,
		Uptime:    uptime,
		Version:   AppVersion,
		Timestamp: time.Now().Unix(),
	}

	body, _ := json.Marshal(hb)
	client := &httpClient10

	for _, peer := range peers {
		if len(peer.Addresses) == 0 {
			continue
		}
		addr := pickBestAddress(peer.Addresses)
		if addr == "" {
			continue
		}

		url := strings.TrimRight(addr, "/") + "/api/network/heartbeat"
		go func(peerID, peerURL string, peerBody []byte) {
			resp, err := client.Post(peerURL, "application/json", bytes.NewReader(peerBody))
			if err != nil {
				slog.Debug("heartbeat failed", "peer", peerID, "url", peerURL, "error", err)
				recordMissedHeartbeat(peerID)
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != 200 {
				slog.Debug("heartbeat non-200", "peer", peerID, "status", resp.StatusCode)
				recordMissedHeartbeat(peerID)
				return
			}

			// Parse gossip response
			var hbResp HeartbeatResponse
			if err := json.NewDecoder(resp.Body).Decode(&hbResp); err == nil {
				processGossipPeers(hbResp.Peers)
			}

			// Mark peer as alive
			recordSuccessfulHeartbeat(peerID)

			// Update peer info in route table
			routeTable.Put(peerID, peer.Name, peer.Addresses)
		}(peer.NodeID, url, body)
	}
}

// recordSuccessfulHeartbeat records a successful heartbeat from a peer.
func recordSuccessfulHeartbeat(nodeID string) {
	state, ok := heartbeatStates[nodeID]
	if !ok {
		state = &peerHeartbeatState{}
		heartbeatStates[nodeID] = state
	}
	state.missedCount = 0
	state.lastHeartbeat = time.Now()

	// Update peer status to online
	if netMgr != nil {
		netMgr.mu.Lock()
		for i, p := range netMgr.config.Peers {
			if p.NodeID == nodeID && p.Status != "online" {
				netMgr.config.Peers[i].Status = "online"
				netMgr.config.Peers[i].LastSeen = time.Now().Format(time.RFC3339)
				netMgr.updateOnlineCount()
				netMgr.doSave()
				slog.Info("peer back online", "node_id", nodeID)
			} else if p.NodeID == nodeID {
				netMgr.config.Peers[i].LastSeen = time.Now().Format(time.RFC3339)
				netMgr.doSave()
			}
		}
		netMgr.mu.Unlock()
	}
}

// recordMissedHeartbeat increments the missed counter for a peer.
func recordMissedHeartbeat(nodeID string) {
	state, ok := heartbeatStates[nodeID]
	if !ok {
		state = &peerHeartbeatState{}
		heartbeatStates[nodeID] = state
	}
	state.missedCount++
}

// checkPeerHealth checks if any peers should be marked offline.
func checkPeerHealth() {
	netMgr.mu.Lock()
	changed := false
	for i, p := range netMgr.config.Peers {
		state, ok := heartbeatStates[p.NodeID]
		if !ok {
			continue
		}
		if state.missedCount >= maxMissedHeartbeats && p.Status != "offline" {
			netMgr.config.Peers[i].Status = "offline"
			changed = true
			slog.Warn("peer marked offline", "node_id", p.NodeID, "missed", state.missedCount)
		}
	}
	if changed {
		netMgr.updateOnlineCount()
		netMgr.doSave()
	}
	netMgr.mu.Unlock()
}

// processGossipPeers processes peers received from a gossip response.
func processGossipPeers(peers []HeartbeatPeerInfo) {
	selfID := netMgr.GetNodeID()
	for _, gp := range peers {
		if gp.NodeID == selfID {
			continue // skip self
		}
		// Check if we already know this peer
		netMgr.mu.RLock()
		known := false
		for _, p := range netMgr.config.Peers {
			if p.NodeID == gp.NodeID {
				known = true
				break
			}
		}
		netMgr.mu.RUnlock()

		if !known && len(gp.Addresses) > 0 {
			// Auto-register discovered peer
			newPeer := PeerInfo{
				NodeID:    gp.NodeID,
				Name:      gp.NodeName,
				Models:    gp.Models,
				Status:    gp.Status,
				LastSeen:  time.Now().Format(time.RFC3339),
				Addresses: gp.Addresses,
				TrustScore: 0.5,
				JoinedAt:  time.Now().Format(time.RFC3339),
			}
			netMgr.AddPeer(newPeer)
			slog.Info("discovered new peer via gossip", "node_id", gp.NodeID, "name", gp.NodeName)
		}

		// Update route table regardless
		if len(gp.Addresses) > 0 {
			routeTable.Put(gp.NodeID, gp.NodeName, gp.Addresses)
		}
	}
}

// ============================================================
// API Handler — Heartbeat Endpoint
// ============================================================

// POST /api/network/heartbeat — receive heartbeat, return gossip peers
func handleNetworkHeartbeat(w http.ResponseWriter, r *http.Request) {
	if netMgr == nil || !netMgr.IsSharedMode() {
		writeError(w, 400, "shared network not active")
		return
	}

	var hb HeartbeatPayload
	if err := readJSON(r, &hb); err != nil {
		writeError(w, 400, "invalid heartbeat payload")
		return
	}

	if hb.NodeID == "" {
		writeError(w, 400, "node_id is required")
		return
	}

	// Update the sender's info in our route table
	if len(hb.Addresses) > 0 {
		routeTable.Put(hb.NodeID, hb.NodeName, hb.Addresses)
	}

	// Update or add the peer
	existingPeer := false
	netMgr.mu.Lock()
	for i, p := range netMgr.config.Peers {
		if p.NodeID == hb.NodeID {
			netMgr.config.Peers[i].Name = hb.NodeName
			netMgr.config.Peers[i].Status = "online"
			netMgr.config.Peers[i].LastSeen = time.Now().Format(time.RFC3339)
			if len(hb.Addresses) > 0 {
				netMgr.config.Peers[i].Addresses = hb.Addresses
			}
			if len(hb.Models) > 0 {
				netMgr.config.Peers[i].Models = hb.Models
			}
			existingPeer = true
			break
		}
	}
	if !existingPeer {
		newPeer := PeerInfo{
			NodeID:     hb.NodeID,
			Name:       hb.NodeName,
			Models:     hb.Models,
			Status:     "online",
			LastSeen:   time.Now().Format(time.RFC3339),
			Addresses:  hb.Addresses,
			TrustScore: 0.5,
			JoinedAt:   time.Now().Format(time.RFC3339),
		}
		netMgr.config.Peers = append(netMgr.config.Peers, newPeer)
		netMgr.config.Stats.TotalPeers = len(netMgr.config.Peers)
	}
	netMgr.updateOnlineCount()
	netMgr.doSave()
	netMgr.mu.Unlock()

	// Record successful heartbeat
	recordSuccessfulHeartbeat(hb.NodeID)

	// Build gossip response: other known peers
	selfID := netMgr.GetNodeID()
	var gossipPeers []HeartbeatPeerInfo
	netMgr.mu.RLock()
	for _, p := range netMgr.config.Peers {
		if p.NodeID == hb.NodeID || p.NodeID == selfID {
			continue // skip the sender and self
		}
		gossipPeers = append(gossipPeers, HeartbeatPeerInfo{
			NodeID:    p.NodeID,
			NodeName:  p.Name,
			Addresses: p.Addresses,
			Models:    p.Models,
			Status:    p.Status,
		})
	}
	netMgr.mu.RUnlock()

	writeJSON(w, 200, HeartbeatResponse{
		Status: "ok",
		Peers:  gossipPeers,
	})
}

// ============================================================
// Public Node Info Endpoint
// ============================================================

// GET /api/node/pubkey — returns this node's public key (for signature verification)
func handleNodePubKey(w http.ResponseWriter, r *http.Request) {
	if node == nil || !node.IsInitialized() {
		writeError(w, 500, "node not initialized")
		return
	}
	writeJSON(w, 200, map[string]string{
		"node_id": netMgr.GetNodeID(),
		"pub_key": node.PubKeyB64(),
	})
}

// GET /api/node/info — returns public node information
func handleNodeInfo(w http.ResponseWriter, r *http.Request) {
	if node == nil || !node.IsInitialized() {
		writeError(w, 500, "node not initialized")
		return
	}

	var uptime int64
	if netMgr != nil && !netMgr.startTime.IsZero() {
		uptime = int64(time.Since(netMgr.startTime).Seconds())
	}

	writeJSON(w, 200, map[string]any{
		"node_id":    netMgr.GetNodeID(),
		"node_name":  netMgr.config.NodeName,
		"pub_key":    node.PubKeyB64(),
		"addresses":  netMgr.collectAddresses(),
		"models":     netMgr.config.SharedModels,
		"uptime":     uptime,
		"version":    AppVersion,
		"mode":       netMgr.config.Mode,
	})
}

// ============================================================
// Contribution Recording
// ============================================================

// RecordContribution records a contribution after a successful relay.
func RecordContribution(fromNodeID string, tokensUsed int64) {
	if netMgr == nil {
		return
	}
	netMgr.mu.Lock()
	defer netMgr.mu.Unlock()

	record := ContribRecord{
		Timestamp:  time.Now().Format(time.RFC3339),
		TokensUsed: tokensUsed,
		Requests:   1,
		FromNodeID: fromNodeID,
	}

	netMgr.config.ContribRecords = append(netMgr.config.ContribRecords, record)

	// Keep only last 1000 records
	if len(netMgr.config.ContribRecords) > 1000 {
		netMgr.config.ContribRecords = netMgr.config.ContribRecords[len(netMgr.config.ContribRecords)-1000:]
	}

	// Add contribution points (1 point per request, or tokens/1000)
	points := tokensUsed / 1000
	if points < 1 {
		points = 1
	}
	netMgr.config.ContribPoints += points

	// Phase 2: Track contribution for unlock state
	if netMgr.config.NodeUnlockStates == nil {
		netMgr.config.NodeUnlockStates = make(map[string]*NodeUnlockState)
	}
	if fromNodeID != "" {
		state, exists := netMgr.config.NodeUnlockStates[fromNodeID]
		if !exists {
			state = &NodeUnlockState{
				NodeID:   fromNodeID,
				Unlocked: false,
			}
			netMgr.config.NodeUnlockStates[fromNodeID] = state
		}
		state.ContribPoints += points
	}

	netMgr.doSave()
}

// httpClient10 is a shared HTTP client with 10s timeout for internal calls.
var httpClient10 = http.Client{Timeout: 10 * time.Second}

// ============================================================
// Bootstrap node auto-registration
// ============================================================

// registerWithBootstraps sends initial heartbeat to all bootstrap nodes.
func registerWithBootstraps() {
	if netMgr == nil || !netMgr.IsSharedMode() {
		return
	}

	netMgr.mu.RLock()
	bootstraps := make([]string, len(netMgr.config.BootstrapNodes))
	copy(bootstraps, netMgr.config.BootstrapNodes)
	netMgr.mu.RUnlock()

	var uptime int64
	if !netMgr.startTime.IsZero() {
		uptime = int64(time.Since(netMgr.startTime).Seconds())
	}

	hb := HeartbeatPayload{
		NodeID:    netMgr.GetNodeID(),
		NodeName:  netMgr.config.NodeName,
		Addresses: netMgr.collectAddresses(),
		Models:    netMgr.config.SharedModels,
		Uptime:    uptime,
		Version:   AppVersion,
		Timestamp: time.Now().Unix(),
	}

	body, _ := json.Marshal(hb)
	client := &httpClient10

	for _, bs := range bootstraps {
		url := strings.TrimRight(bs, "/") + "/api/network/heartbeat"
		go func(bsURL string, bsBody []byte) {
			resp, err := client.Post(bsURL, "application/json", bytes.NewReader(bsBody))
			if err != nil {
				slog.Debug("bootstrap registration failed", "url", bsURL, "error", err)
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode == 200 {
				var hbResp HeartbeatResponse
				if err := json.NewDecoder(resp.Body).Decode(&hbResp); err == nil {
					processGossipPeers(hbResp.Peers)
					slog.Info("registered with bootstrap node", "url", bsURL, "discovered_peers", len(hbResp.Peers))
				}
			}
		}(url, body)
	}
}
