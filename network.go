package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ============================================================
// Network Mode & Data Models
// ============================================================

type NetworkMode string

const (
	NetworkModePersonal NetworkMode = "personal" // 个人模式（默认）
	NetworkModeShared   NetworkMode = "shared"   // 共享网络模式
)

const (
	p2pNodeIDPrefix = "mmx-"
	maxRelayHops    = 3
	routeTTL        = 10 * time.Minute // 路由条目 TTL
	refreshInterval = 5 * time.Minute  // 地址刷新间隔
)

// ContribRecord tracks individual contribution events (Phase 2)
type ContribRecord struct {
	Timestamp  string `json:"timestamp"`
	TokensUsed int64  `json:"tokens_used"`
	Requests   int64  `json:"requests"`
	FromNodeID string `json:"from_node_id"`
}

// NetworkConfig holds all shared network configuration
type NetworkConfig struct {
	Mode              NetworkMode     `json:"mode"`
	ConsentAccepted   bool            `json:"consent_accepted"`
	ConsentTime       string          `json:"consent_time"`
	NodeName          string          `json:"node_name"`
	NodeID            string          `json:"node_id"`
	BootstrapNodes    []string        `json:"bootstrap_nodes"`
	SharedModels      []string        `json:"shared_models"`
	MaxDailyRequests  int             `json:"max_daily_requests"`
	ContribPoints     int64           `json:"contrib_points"`
	ContribRecords    []ContribRecord `json:"contrib_records"`
	Peers             []PeerInfo      `json:"peers"`
	Stats             NetworkStats    `json:"stats"`
	Addresses         []string        `json:"addresses"`
	LastAddressUpdate string          `json:"last_address_update"`
	RelayEnabled      bool            `json:"relay_enabled"`

	// Phase 2 Economic Model
	TrialPool         TrialPool                       `json:"trial_pool"`
	NodeUnlockStates  map[string]*NodeUnlockState     `json:"node_unlock_states"`
}

// PeerInfo represents a connected peer in the shared network
type PeerInfo struct {
	NodeID     string   `json:"node_id"`
	Name       string   `json:"name"`
	Region     string   `json:"region"`
	Models     []string `json:"models"`
	Status     string   `json:"status"`
	LastSeen   string   `json:"last_seen"`
	TrustScore float64  `json:"trust_score"`
	JoinedAt   string   `json:"joined_at"`
	Addresses  []string `json:"addresses,omitempty"`
	Unlocked   bool     `json:"unlocked"`
}

// NetworkStats holds network statistics
type NetworkStats struct {
	TotalPeers        int    `json:"total_peers"`
	OnlinePeers       int    `json:"online_peers"`
	TotalModelsShared int    `json:"total_models_shared"`
	RequestsRelayed   int64  `json:"requests_relayed"`
	RequestsReceived  int64  `json:"requests_received"`
	RelaySuccess      int64  `json:"relay_success"`
	RelayFailed       int64  `json:"relay_failed"`
	UptimeSeconds     int64  `json:"uptime_seconds"`
	JoinedAt          string `json:"joined_at"`
}

// DisclaimerSection for the disclaimer endpoint
type DisclaimerSection struct {
	Heading string `json:"heading"`
	Content string `json:"content"`
	IsRisk  bool   `json:"is_risk,omitempty"`
}

// DisclaimerResponse is the response for the disclaimer endpoint
type DisclaimerResponse struct {
	Title            string              `json:"title"`
	Sections         []DisclaimerSection `json:"sections"`
	ConfirmationText string              `json:"confirmation_text"`
}

// ============================================================
// Route Table — Phase 1 simplified DHT (replaced by Kademlia in Phase 2)
// ============================================================

// RouteEntry maps a NodeID to its reachable addresses
type RouteEntry struct {
	NodeID    string    `json:"node_id"`
	NodeName  string    `json:"node_name"`
	Addresses []string  `json:"addresses"`
	Status    string    `json:"status"` // online/offline/degraded
	UpdatedAt time.Time `json:"updated_at"`
}

// RouteTable is a simplified DHT routing table (Phase 1)
// Phase 2 will replace this with libp2p Kademlia
type RouteTable struct {
	mu      sync.RWMutex
	entries map[string]*RouteEntry
}

var routeTable *RouteTable

func initRouteTable() *RouteTable {
	return &RouteTable{entries: make(map[string]*RouteEntry)}
}

// Put adds or updates a route entry
func (rt *RouteTable) Put(nodeID, nodeName string, addresses []string) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.entries[nodeID] = &RouteEntry{
		NodeID:    nodeID,
		NodeName:  nodeName,
		Addresses: addresses,
		Status:    "online",
		UpdatedAt: time.Now(),
	}
}

// Get looks up a route entry by NodeID
func (rt *RouteTable) Get(nodeID string) *RouteEntry {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	e, ok := rt.entries[nodeID]
	if !ok {
		return nil
	}
	// Check TTL
	if time.Since(e.UpdatedAt) > routeTTL {
		return nil // expired
	}
	// Return copy
	cp := *e
	addrs := make([]string, len(e.Addresses))
	copy(addrs, e.Addresses)
	cp.Addresses = addrs
	return &cp
}

// Remove deletes a route entry
func (rt *RouteTable) Remove(nodeID string) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	delete(rt.entries, nodeID)
}

// GetAll returns all non-expired entries
func (rt *RouteTable) GetAll() []*RouteEntry {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	now := time.Now()
	result := make([]*RouteEntry, 0, len(rt.entries))
	for _, e := range rt.entries {
		if now.Sub(e.UpdatedAt) > routeTTL {
			continue
		}
		cp := *e
		addrs := make([]string, len(e.Addresses))
		copy(addrs, e.Addresses)
		cp.Addresses = addrs
		result = append(result, &cp)
	}
	return result
}

// PurgeExpired removes stale entries
func (rt *RouteTable) PurgeExpired() int {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	now := time.Now()
	purged := 0
	for id, e := range rt.entries {
		if now.Sub(e.UpdatedAt) > routeTTL {
			delete(rt.entries, id)
			purged++
		}
	}
	return purged
}

// Count returns the number of active entries
func (rt *RouteTable) Count() int {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	return len(rt.entries)
}

// ============================================================
// NetworkManager
// ============================================================

type NetworkManager struct {
	mu          sync.RWMutex
	config      NetworkConfig
	dataPath    string
	startTime   time.Time
	stopRefresh chan struct{}
}

var netMgr *NetworkManager

func initNetworkManager(dataDir string) {
	netMgr = &NetworkManager{
		dataPath: filepath.Join(dataDir, "network.json"),
		config: NetworkConfig{
			Mode:             NetworkModePersonal,
			ConsentAccepted:  false,
			BootstrapNodes:   []string{},
			SharedModels:     []string{},
			Peers:            []PeerInfo{},
			MaxDailyRequests: 1000,
			Addresses:        []string{},
			RelayEnabled:     true, // default on when in shared mode
		},
	}
	netMgr.load()
	routeTable = initRouteTable()

	// Re-register self in route table if we have addresses
	if netMgr.config.NodeID != "" && len(netMgr.config.Addresses) > 0 {
		routeTable.Put(netMgr.config.NodeID, netMgr.config.NodeName, netMgr.config.Addresses)
	}

	slog.Info("network manager initialized", "mode", netMgr.config.Mode, "node_id", netMgr.config.NodeID)
}

func (nm *NetworkManager) load() {
	nm.mu.Lock()
	defer nm.mu.Unlock()
	b, err := os.ReadFile(nm.dataPath)
	if err != nil {
		return
	}
	json.Unmarshal(b, &nm.config)
	if nm.config.BootstrapNodes == nil {
		nm.config.BootstrapNodes = []string{}
	}
	if nm.config.SharedModels == nil {
		nm.config.SharedModels = []string{}
	}
	if nm.config.Peers == nil {
		nm.config.Peers = []PeerInfo{}
	}
	if nm.config.Addresses == nil {
		nm.config.Addresses = []string{}
	}
	if nm.config.NodeUnlockStates == nil {
		nm.config.NodeUnlockStates = make(map[string]*NodeUnlockState)
	}
	if nm.config.TrialPool.TrialKeys == nil {
		nm.config.TrialPool.TrialKeys = make([]*TrialKeyInfo, 0)
	}
}

func (nm *NetworkManager) save() {
	nm.mu.RLock()
	defer nm.mu.RUnlock()
	nm.doSave()
}

func (nm *NetworkManager) doSave() {
	os.MkdirAll(filepath.Dir(nm.dataPath), 0755)
	b, _ := json.MarshalIndent(nm.config, "", "  ")
	os.WriteFile(nm.dataPath, b, 0600)
}

// Init loads config and derives NodeID. Starts refresh loop if shared mode.
func (nm *NetworkManager) Init() error {
	nm.load()

	// Derive P2P NodeID from Ed25519 identity
	if nm.config.NodeID == "" && node != nil && node.IsInitialized() {
		nm.config.NodeID = DeriveP2PNodeID()
		nm.doSave()
		slog.Info("derived P2P NodeID", "node_id", nm.config.NodeID)
	}

	if nm.config.Mode == NetworkModeShared && nm.config.ConsentAccepted {
		nm.startTime = time.Now()
		nm.registerSelf()
		nm.startRefreshLoop()
		slog.Info("shared network mode active", "node_id", nm.config.NodeID)
	}
	return nil
}

// DeriveP2PNodeID creates deterministic P2P NodeID from Ed25519 public key.
// Format: mmx- + hex(sha256(pubkey)[:16]) = mmx- + 32 hex chars = 36 total
func DeriveP2PNodeID() string {
	if node == nil || node.pubKey == nil {
		return ""
	}
	hash := sha256.Sum256(node.pubKey)
	return p2pNodeIDPrefix + hex.EncodeToString(hash[:16])
}

// registerSelf registers this node's addresses in the route table
func (nm *NetworkManager) registerSelf() {
	nm.mu.RLock()
	nodeID := nm.config.NodeID
	nodeName := nm.config.NodeName
	nm.mu.RUnlock()

	if nodeID == "" {
		return
	}
	addresses := nm.collectAddresses()

	nm.mu.Lock()
	nm.config.Addresses = addresses
	nm.config.LastAddressUpdate = time.Now().Format(time.RFC3339)
	nm.mu.Unlock()

	routeTable.Put(nodeID, nodeName, addresses)
	slog.Info("registered self in route table", "node_id", nodeID, "addresses", addresses)
}

// collectAddresses gathers all reachable URLs for this node
func (nm *NetworkManager) collectAddresses() []string {
	var addrs []string
	if u := cfg.Get("tunnel_url", ""); u != "" {
		addrs = append(addrs, u)
	}
	if d := cfg.Get("tunnel_domain", ""); d != "" {
		addrs = append(addrs, "https://"+d)
	}
	port := cfg.Get("service_port", "8000")
	addrs = append(addrs, fmt.Sprintf("http://localhost:%s", port))
	return addrs
}

// startRefreshLoop periodically refreshes addresses and purges stale routes
func (nm *NetworkManager) startRefreshLoop() {
	nm.stopRefresh = make(chan struct{})
	go func() {
		ticker := time.NewTicker(refreshInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				nm.registerSelf()
				purged := routeTable.PurgeExpired()
				if purged > 0 {
					slog.Debug("purged expired route entries", "count", purged)
				}
			case <-nm.stopRefresh:
				return
			}
		}
	}()
}

func (nm *NetworkManager) stopRefreshLoop() {
	if nm.stopRefresh != nil {
		close(nm.stopRefresh)
		nm.stopRefresh = nil
	}
}

// EnableSharedNetwork activates shared network (requires consent)
func (nm *NetworkManager) EnableSharedNetwork() error {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	if !nm.config.ConsentAccepted {
		return fmt.Errorf("consent not accepted")
	}
	if nm.config.NodeID == "" {
		if node != nil && node.IsInitialized() {
			nm.config.NodeID = DeriveP2PNodeID()
		}
		if nm.config.NodeID == "" {
			return fmt.Errorf("node identity not initialized")
		}
	}
	if nm.config.NodeName == "" {
		suffix := nm.config.NodeID
		if len(suffix) > 8 {
			suffix = suffix[4:8]
		}
		nm.config.NodeName = "node-" + suffix
	}

	nm.config.Mode = NetworkModeShared
	nm.config.Stats.JoinedAt = time.Now().Format(time.RFC3339)
	nm.startTime = time.Now()

	// Initialize unlock state for self
	if nm.config.NodeUnlockStates == nil {
		nm.config.NodeUnlockStates = make(map[string]*NodeUnlockState)
	}
	nm.config.NodeUnlockStates[nm.config.NodeID] = &NodeUnlockState{
		NodeID:   nm.config.NodeID,
		Unlocked: true, // self is always unlocked
		ContribPoints: nm.config.ContribPoints,
		Progress: 1.0,
	}

	nm.doSave()

	go nm.registerSelf()
	nm.startRefreshLoop()

	// Auto-create trial pool for new nodes (Phase 2)
	go func() {
		time.Sleep(2 * time.Second) // wait for registration
		if keyStore != nil && len(nm.config.TrialPool.TrialKeys) == 0 {
			if err := nm.CreateTrialPoolForNode(nm.config.NodeID, DefaultTrialQuota); err != nil {
				slog.Warn("failed to create trial pool", "error", err)
			} else {
				slog.Info("trial pool created for node", "node_id", nm.config.NodeID)
			}
		}
	}()

	slog.Info("shared network enabled", "node_id", nm.config.NodeID, "name", nm.config.NodeName)
	return nil
}

// DisableSharedNetwork returns to personal mode
func (nm *NetworkManager) DisableSharedNetwork() error {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	nm.stopRefreshLoop()

	if nm.config.NodeID != "" {
		routeTable.Remove(nm.config.NodeID)
	}

	nm.config.Mode = NetworkModePersonal
	nm.config.Peers = []PeerInfo{}
	nm.config.Stats.OnlinePeers = 0
	nm.config.Addresses = []string{}
	nm.doSave()

	slog.Info("shared network disabled")
	return nil
}

// RecordConsent records user consent
func (nm *NetworkManager) RecordConsent() error {
	nm.mu.Lock()
	defer nm.mu.Unlock()
	nm.config.ConsentAccepted = true
	nm.config.ConsentTime = time.Now().Format(time.RFC3339)
	nm.doSave()
	return nil
}

// GetStatus returns current network status (thread-safe, read-only copy)
func (nm *NetworkManager) GetStatus() map[string]any {
	nm.mu.RLock()
	defer nm.mu.RUnlock()

	uptime := int64(0)
	if nm.config.Mode == NetworkModeShared && !nm.startTime.IsZero() {
		uptime = int64(time.Since(nm.startTime).Seconds())
	}

	// Build relay consumer URL hint
	relayURL := ""
	if nm.config.NodeID != "" && len(nm.config.Addresses) > 0 {
		// Pick the first public address as relay hint
		for _, a := range nm.config.Addresses {
			if strings.HasPrefix(a, "https://") {
				relayURL = fmt.Sprintf("%s/network/%s/v1", a, nm.config.NodeID)
				break
			}
		}
		if relayURL == "" && len(nm.config.Addresses) > 0 {
			relayURL = fmt.Sprintf("%s/network/%s/v1", nm.config.Addresses[0], nm.config.NodeID)
		}
	}

	return map[string]any{
		"mode":               nm.config.Mode,
		"consent_accepted":   nm.config.ConsentAccepted,
		"consent_time":       nm.config.ConsentTime,
		"node_name":          nm.config.NodeName,
		"node_id":            nm.config.NodeID,
		"shared_models":      nm.config.SharedModels,
		"max_daily_requests": nm.config.MaxDailyRequests,
		"contrib_points":     nm.config.ContribPoints,
		"bootstrap_nodes":    nm.config.BootstrapNodes,
		"stats":              nm.config.Stats,
		"peers_count":        len(nm.config.Peers),
		"addresses":          nm.config.Addresses,
		"uptime_seconds":     uptime,
		"relay_enabled":      nm.config.RelayEnabled,
		"relay_consumer_url": relayURL,
		"route_table_size":   routeTable.Count(),

		// Phase 2 Economic Model
		"trial_pool_count":  len(nm.config.TrialPool.TrialKeys),
		"unlock_states":     len(nm.config.NodeUnlockStates),
	}
}

func (nm *NetworkManager) IsSharedMode() bool {
	nm.mu.RLock()
	defer nm.mu.RUnlock()
	return nm.config.Mode == NetworkModeShared
}

func (nm *NetworkManager) GetNodeID() string {
	nm.mu.RLock()
	defer nm.mu.RUnlock()
	return nm.config.NodeID
}

// AddPeer adds/updates a peer and registers in route table
func (nm *NetworkManager) AddPeer(peer PeerInfo) error {
	nm.mu.Lock()
	defer nm.mu.Unlock()
	if nm.config.Mode != NetworkModeShared {
		return fmt.Errorf("shared network not active")
	}
	for i, p := range nm.config.Peers {
		if p.NodeID == peer.NodeID {
			// Preserve unlock state
			peer.Unlocked = p.Unlocked
			nm.config.Peers[i] = peer
			nm.doSave()
			if len(peer.Addresses) > 0 {
				routeTable.Put(peer.NodeID, peer.Name, peer.Addresses)
			}
			return nil
		}
	}
	// New peer — init unlock state
	if nm.config.NodeUnlockStates == nil {
		nm.config.NodeUnlockStates = make(map[string]*NodeUnlockState)
	}
	if _, exists := nm.config.NodeUnlockStates[peer.NodeID]; !exists {
		nm.config.NodeUnlockStates[peer.NodeID] = &NodeUnlockState{
			NodeID:   peer.NodeID,
			Unlocked: false,
		}
	}
	nm.config.Peers = append(nm.config.Peers, peer)
	nm.config.Stats.TotalPeers = len(nm.config.Peers)
	nm.updateOnlineCount()
	nm.doSave()
	if len(peer.Addresses) > 0 {
		routeTable.Put(peer.NodeID, peer.Name, peer.Addresses)
	}
	return nil
}

// RemovePeer removes a peer by node ID
func (nm *NetworkManager) RemovePeer(nodeID string) error {
	nm.mu.Lock()
	defer nm.mu.Unlock()
	if nm.config.Mode != NetworkModeShared {
		return fmt.Errorf("shared network not active")
	}
	found := false
	newPeers := []PeerInfo{}
	for _, p := range nm.config.Peers {
		if p.NodeID == nodeID {
			found = true
			continue
		}
		newPeers = append(newPeers, p)
	}
	if !found {
		return fmt.Errorf("peer not found: %s", nodeID)
	}
	nm.config.Peers = newPeers
	nm.config.Stats.TotalPeers = len(nm.config.Peers)
	nm.updateOnlineCount()
	nm.doSave()
	return nil
}

func (nm *NetworkManager) updateOnlineCount() {
	count := 0
	for _, p := range nm.config.Peers {
		if p.Status == "online" {
			count++
		}
	}
	nm.config.Stats.OnlinePeers = count
}

func (nm *NetworkManager) GetPeers() []PeerInfo {
	nm.mu.RLock()
	defer nm.mu.RUnlock()
	peers := make([]PeerInfo, len(nm.config.Peers))
	copy(peers, nm.config.Peers)
	return peers
}

// UpdateConfig updates network configuration
func (nm *NetworkManager) UpdateConfig(nodeName string, sharedModels []string, maxDaily int, relayEnabled *bool) error {
	nm.mu.Lock()
	defer nm.mu.Unlock()
	if nm.config.Mode != NetworkModeShared {
		return fmt.Errorf("shared network not active")
	}
	if nodeName != "" {
		nm.config.NodeName = nodeName
	}
	if sharedModels != nil {
		nm.config.SharedModels = sharedModels
		nm.config.Stats.TotalModelsShared = len(sharedModels)
	}
	if maxDaily > 0 {
		nm.config.MaxDailyRequests = maxDaily
	}
	if relayEnabled != nil {
		nm.config.RelayEnabled = *relayEnabled
	}
	nm.doSave()
	return nil
}

// RecordRelayResult records a relay outcome
func (nm *NetworkManager) RecordRelayResult(success bool) {
	nm.mu.Lock()
	defer nm.mu.Unlock()
	nm.config.Stats.RequestsRelayed++
	if success {
		nm.config.Stats.RelaySuccess++
	} else {
		nm.config.Stats.RelayFailed++
	}
}

// RecordReceived records an incoming relay request
func (nm *NetworkManager) RecordReceived() {
	nm.mu.Lock()
	defer nm.mu.Unlock()
	nm.config.Stats.RequestsReceived++
}

// RefreshAddresses re-collects addresses (called on tunnel change)
func (nm *NetworkManager) RefreshAddresses() {
	if !nm.IsSharedMode() {
		return
	}
	nm.registerSelf()
}

// GetDisclaimer returns the disclaimer text
func GetDisclaimer() DisclaimerResponse {
	return DisclaimerResponse{
		Title: "共享网络使用须知",
		Sections: []DisclaimerSection{
			{
				Heading: "什么是共享网络？",
				Content: "ModelMux Agent 本质上是一个 AI 智能代理（Agent）——和你使用的任何 AI Agent 没有区别：持有 API Key，向上游模型服务商发送请求，获取响应。\n\n共享网络只是在这个 Agent 的基础上增加了一个可选功能：将你闲置的模型调用能力分享给网络中的其他用户，同时也可以使用他人分享的模型能力。每个节点都可以作为 relay 为他人转发请求，形成去中心化的 P2P 网络。\n\n这和你自己部署一个 Agent 来调用 API 在本质上是相同的——区别仅在于 prompt 来自谁。对上游服务商而言，请求来自同一个 API Key，消耗的是同一个账户配额。",
			},
			{
				Heading: "启用后将发生什么？",
				Content: "• 您的节点将对外公开（节点名称、可用模型列表、大致地区）\n• 您的节点自动成为 relay 节点，可以为其他节点转发请求\n• 消费者可以通过任意 relay 节点使用 URL 格式 https://{relay地址}/network/{NodeID}/v1 访问目标节点\n• 您的 API Key 不会被暴露，请求通过 relay 反向代理转发\n• 您将开始积累贡献积分（积分仅为参与激励，不可变现、不可交易）",
			},
			{
				Heading: "关于模型能力的安全责任",
				Content: "• 所有通过本网络流转的 AI 请求，最终都到达您配置的上游模型服务商（如 OpenAI、Anthropic 等）\n• 模型能力的合法性、安全性由上游服务商负责保障——您使用正规渠道购买的 API Key，通过本网络转发请求，与直接调用并无本质区别\n• 本软件是去中心化工具，不是平台、不是服务商。每个节点使用自己的 API Key，对自己的账户行为负责\n• 不存在\"转售\"行为——每个节点都是在用自己的 Key 转发请求，和用 Agent 调用 API 完全一样",
			},
			{
				Heading: "⚠️ 风险警告",
				IsRisk:  true,
				Content: "• 部分 AI 平台的服务条款可能限制 API 代理行为，启用共享网络可能导致您的 API 账号受限\n• 系统已实施速率限制和行为模拟，但无法完全消除平台检测风险\n• 您分享的计算资源可能被他人生成不当内容，您需承担相应平台的风控后果\n• 不同区域的法律法规可能对 AI 服务的使用有不同要求\n• 贡献积分仅作为参与网络的激励记录，不具有任何货币价值，不可交易或变现",
			},
		},
		ConfirmationText: "我已阅读并理解以上说明，自愿承担相关风险",
	}
}

// ============================================================
// API Handlers — Network Management
// ============================================================

func handleNetworkStatus(w http.ResponseWriter, r *http.Request) {
	if netMgr == nil {
		writeJSON(w, 200, map[string]any{"mode": "personal", "consent_accepted": false})
		return
	}
	writeJSON(w, 200, netMgr.GetStatus())
}

func handleNetworkConsent(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Accepted bool `json:"accepted"`
	}
	if err := readJSON(r, &body); err != nil || !body.Accepted {
		writeError(w, 400, "accepted must be true")
		return
	}
	if err := netMgr.RecordConsent(); err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"status": "consent_recorded", "consent_time": netMgr.config.ConsentTime})
}

func handleNetworkDisclaimer(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, GetDisclaimer())
}

func handleNetworkEnable(w http.ResponseWriter, r *http.Request) {
	if err := netMgr.EnableSharedNetwork(); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"status": "enabled", "mode": "shared", "node_id": netMgr.config.NodeID})
}

func handleNetworkDisable(w http.ResponseWriter, r *http.Request) {
	if err := netMgr.DisableSharedNetwork(); err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"status": "disabled", "mode": "personal"})
}

func handleNetworkConfigUpdate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		NodeName     string   `json:"node_name"`
		SharedModels []string `json:"shared_models"`
		MaxDaily     int      `json:"max_daily_requests"`
		RelayEnabled *bool    `json:"relay_enabled,omitempty"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, 400, "invalid request body")
		return
	}
	if err := netMgr.UpdateConfig(body.NodeName, body.SharedModels, body.MaxDaily, body.RelayEnabled); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"status": "updated"})
}

func handleNetworkPeers(w http.ResponseWriter, r *http.Request) {
	if !netMgr.IsSharedMode() {
		writeJSON(w, 200, map[string]any{"peers": []PeerInfo{}, "message": "shared network not active"})
		return
	}
	writeJSON(w, 200, map[string]any{"peers": netMgr.GetPeers()})
}

func handleNetworkAddPeer(w http.ResponseWriter, r *http.Request) {
	var peer PeerInfo
	if err := readJSON(r, &peer); err != nil {
		writeError(w, 400, "invalid request body")
		return
	}
	if peer.NodeID == "" {
		writeError(w, 400, "node_id is required")
		return
	}
	if peer.Status == "" {
		peer.Status = "online"
	}
	if peer.LastSeen == "" {
		peer.LastSeen = time.Now().Format(time.RFC3339)
	}
	if peer.TrustScore == 0 {
		peer.TrustScore = 0.5
	}
	if err := netMgr.AddPeer(peer); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"status": "added", "peer": peer})
}

func handleNetworkRemovePeer(w http.ResponseWriter, r *http.Request) {
	nodeID := r.PathValue("id")
	if nodeID == "" {
		writeError(w, 400, "peer id required")
		return
	}
	if err := netMgr.RemovePeer(nodeID); err != nil {
		writeError(w, 404, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"status": "removed", "node_id": nodeID})
}

// GET /api/network/resolve/{id} — resolve NodeID to addresses
func handleNetworkResolve(w http.ResponseWriter, r *http.Request) {
	nodeID := r.PathValue("id")
	if nodeID == "" {
		writeError(w, 400, "node_id required")
		return
	}
	if !strings.HasPrefix(nodeID, p2pNodeIDPrefix) {
		writeError(w, 400, "invalid node_id format; must start with '"+p2pNodeIDPrefix+"'")
		return
	}
	entry := routeTable.Get(nodeID)
	if entry == nil {
		writeJSON(w, 404, map[string]any{"node_id": nodeID, "addresses": []string{}, "status": "not_found"})
		return
	}
	writeJSON(w, 200, map[string]any{
		"node_id":    entry.NodeID,
		"node_name":  entry.NodeName,
		"addresses":  entry.Addresses,
		"status":     entry.Status,
		"updated_at": entry.UpdatedAt.Format(time.RFC3339),
	})
}

// GET /api/network/routes — list all route table entries (admin)
func handleNetworkRoutes(w http.ResponseWriter, r *http.Request) {
	entries := routeTable.GetAll()
	writeJSON(w, 200, map[string]any{"entries": entries, "count": len(entries)})
}

// ============================================================
// Phase 2 Economic Model — Trial Pool & Node Unlock
// ============================================================

// TrialPool stores trial key information per node
type TrialPool struct {
	TrialKeys []*TrialKeyInfo `json:"trial_keys"`
}

// NodeUnlockState tracks a node's unlock progress
type NodeUnlockState struct {
	NodeID            string  `json:"node_id"`
	Unlocked          bool    `json:"unlocked"`
	ContribPoints     int64   `json:"contrib_points"`
	ThresholdRequired int64   `json:"threshold_required"`
	Progress          float64 `json:"progress"` // 0.0 to 1.0
	UnlockedAt        string  `json:"unlocked_at,omitempty"`
}

// AddTrialKey adds a trial key to the pool
func (nm *NetworkManager) AddTrialKey(info *TrialKeyInfo) {
	nm.mu.Lock()
	defer nm.mu.Unlock()
	if nm.config.TrialPool.TrialKeys == nil {
		nm.config.TrialPool.TrialKeys = make([]*TrialKeyInfo, 0)
	}
	nm.config.TrialPool.TrialKeys = append(nm.config.TrialPool.TrialKeys, info)
	nm.doSave()
}

// GetTrialKeys returns all trial keys
func (nm *NetworkManager) GetTrialKeys() []*TrialKeyInfo {
	nm.mu.RLock()
	defer nm.mu.RUnlock()
	if nm.config.TrialPool.TrialKeys == nil {
		return make([]*TrialKeyInfo, 0)
	}
	return nm.config.TrialPool.TrialKeys
}

// CreateTrialPoolForNode creates a trial pool when a node joins
func (nm *NetworkManager) CreateTrialPoolForNode(nodeID string, quota int64) error {
	if quota <= 0 {
		quota = DefaultTrialQuota
	}
	if keyStore == nil {
		return fmt.Errorf("key store not initialized")
	}
	_, _, err := keyStore.IssueTrialKey(nodeID, quota)
	return err
}

// ============================================================
// Dynamic Threshold Unlock (Cold Start)
// ============================================================

// CalculateUnlockThreshold calculates the dynamic unlock threshold.
// threshold = networkAverageContribution * 0.3 * networkScaleFactor
func (nm *NetworkManager) CalculateUnlockThreshold() int64 {
	nm.mu.RLock()
	defer nm.mu.RUnlock()

	// Calculate total and average contribution
	totalContrib := nm.config.ContribPoints
	nodeCount := len(nm.config.Peers) + 1 // +1 for self

	// Add approximate peer contributions
	for _, peer := range nm.config.Peers {
		totalContrib += int64(peer.TrustScore * 1000) // approximate
	}

	avgContrib := int64(0)
	if nodeCount > 0 {
		avgContrib = totalContrib / int64(nodeCount)
	}

	// Network scale factor
	scaleFactor := 1.0
	if nodeCount > 1000 {
		scaleFactor = 0.5
	} else if nodeCount > 100 {
		scaleFactor = 0.8
	}

	threshold := int64(float64(avgContrib) * 0.3 * scaleFactor)
	if threshold < 10 {
		threshold = 10 // minimum threshold
	}
	return threshold
}

// CheckAndUnlockNode checks if a node has enough contributions to unlock
func (nm *NetworkManager) CheckAndUnlockNode(nodeID string) bool {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	state, exists := nm.config.NodeUnlockStates[nodeID]
	if !exists {
		return false
	}

	if state.Unlocked {
		return true
	}

	threshold := nm.calculateUnlockThresholdLocked()
	if state.ContribPoints >= threshold {
		state.Unlocked = true
		state.Progress = 1.0
		state.ThresholdRequired = threshold
		state.UnlockedAt = time.Now().Format(time.RFC3339)
		nm.doSave()
		slog.Info("node unlocked", "node_id", nodeID, "contrib", state.ContribPoints, "threshold", threshold)
		return true
	}

	state.ThresholdRequired = threshold
	if threshold > 0 {
		state.Progress = float64(state.ContribPoints) / float64(threshold)
	}
	nm.doSave()
	return false
}

// calculateUnlockThresholdLocked calculates threshold while lock is held
func (nm *NetworkManager) calculateUnlockThresholdLocked() int64 {
	totalContrib := int64(0)
	for _, state := range nm.config.NodeUnlockStates {
		totalContrib += state.ContribPoints
	}
	nodeCount := len(nm.config.NodeUnlockStates)
	if nodeCount == 0 {
		nodeCount = 1
	}

	avgContrib := totalContrib / int64(nodeCount)

	scaleFactor := 1.0
	if nodeCount > 1000 {
		scaleFactor = 0.5
	} else if nodeCount > 100 {
		scaleFactor = 0.8
	}

	threshold := int64(float64(avgContrib) * 0.3 * scaleFactor)
	if threshold < 10 {
		threshold = 10
	}
	return threshold
}

// RecordContributionWithUnlock records contribution and checks unlock
func (nm *NetworkManager) RecordContributionWithUnlock(nodeID string, points int64) {
	nm.mu.Lock()
	state, exists := nm.config.NodeUnlockStates[nodeID]
	if !exists {
		state = &NodeUnlockState{
			NodeID:        nodeID,
			Unlocked:      false,
			ContribPoints: 0,
		}
		nm.config.NodeUnlockStates[nodeID] = state
	}
	state.ContribPoints += points
	nm.mu.Unlock()

	nm.CheckAndUnlockNode(nodeID)
}

// IsNodeUnlocked checks if a node is unlocked
func (nm *NetworkManager) IsNodeUnlocked(nodeID string) bool {
	nm.mu.RLock()
	defer nm.mu.RUnlock()

	// Self is always unlocked if in shared mode
	if nodeID == nm.config.NodeID {
		return true
	}

	state, exists := nm.config.NodeUnlockStates[nodeID]
	if !exists {
		return false
	}
	return state.Unlocked
}

// GetUnlockStatus returns unlock status for this node and all known nodes
func (nm *NetworkManager) GetUnlockStatus() map[string]any {
	nm.mu.RLock()
	defer nm.mu.RUnlock()

	threshold := nm.calculateUnlockThresholdLocked()

	states := make(map[string]*NodeUnlockState)
	for k, v := range nm.config.NodeUnlockStates {
		cp := *v
		if !cp.Unlocked && threshold > 0 {
			cp.Progress = float64(cp.ContribPoints) / float64(threshold)
		}
		cp.ThresholdRequired = threshold
		states[k] = &cp
	}

	selfID := nm.config.NodeID
	selfState, exists := states[selfID]
	if !exists {
		selfState = &NodeUnlockState{
			NodeID:            selfID,
			Unlocked:          true, // self always unlocked
			ContribPoints:     nm.config.ContribPoints,
			ThresholdRequired: threshold,
			Progress:          1.0,
		}
	}

	nodeCount := len(nm.config.NodeUnlockStates)
	if nodeCount == 0 {
		nodeCount = 1
	}
	scaleFactor := 1.0
	if nodeCount > 1000 {
		scaleFactor = 0.5
	} else if nodeCount > 100 {
		scaleFactor = 0.8
	}

	return map[string]any{
		"self":            selfState,
		"all_states":      states,
		"threshold":       threshold,
		"node_count":      nodeCount,
		"scale_factor":    scaleFactor,
	}
}

// InitNodeUnlockState initializes unlock state for a new node
func (nm *NetworkManager) InitNodeUnlockState(nodeID string) {
	nm.mu.Lock()
	defer nm.mu.Unlock()
	if nm.config.NodeUnlockStates == nil {
		nm.config.NodeUnlockStates = make(map[string]*NodeUnlockState)
	}
	if _, exists := nm.config.NodeUnlockStates[nodeID]; !exists {
		nm.config.NodeUnlockStates[nodeID] = &NodeUnlockState{
			NodeID:   nodeID,
			Unlocked: false,
		}
		nm.doSave()
	}
}
