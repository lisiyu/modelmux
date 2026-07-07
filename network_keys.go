package main

import (
	"crypto/ed25519"
	"encoding/base64"
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
// Signed Key System (Phase 2)
// ============================================================
//
// Key format: mk_{consumer_id}.{payload_base64}.{signature_hex}
//
// The issuer (this node) signs the payload with its Ed25519 private key.
// Consumers present the mk_ key in the Authorization header when making
// relay requests. The target node verifies the signature using the
// issuer's public key (obtained from the route table or bootstrap).

// KeyPayload is the JSON structure embedded in a signed key.
type KeyPayload struct {
	Sub    string   `json:"sub"`    // consumer_id
	Iss    string   `json:"iss"`    // issuer NodeID
	Quota  int64    `json:"quota"`  // total allowed requests
	Used   int64    `json:"used"`   // requests consumed so far
	Models []string `json:"models"` // allowed model list
	Iat    int64    `json:"iat"`    // issued-at unix timestamp
	Exp    int64    `json:"exp"`    // expiration unix timestamp
}

// IssuedKey is the on-disk record for a key issued by this node.
type IssuedKey struct {
	ConsumerID  string   `json:"consumer_id"`
	Key         string   `json:"key"` // full mk_ format
	Payload     KeyPayload `json:"payload"`
	IssuedAt    string   `json:"issued_at"`
	Revoked     bool     `json:"revoked"`
}

// KeyStore manages all keys issued by this node.
type KeyStore struct {
	mu       sync.RWMutex
	keys     map[string]*IssuedKey // keyed by consumer_id
	dataPath string
}

var keyStore *KeyStore

func initKeyStore(dataDir string) {
	keyStore = &KeyStore{
		keys:     make(map[string]*IssuedKey),
		dataPath: filepath.Join(dataDir, "issued_keys.json"),
	}
	keyStore.load()
	slog.Info("key store initialized", "issued_keys", len(keyStore.keys))
}

func (ks *KeyStore) load() {
	b, err := os.ReadFile(ks.dataPath)
	if err != nil {
		return
	}
	var issued []*IssuedKey
	if err := json.Unmarshal(b, &issued); err != nil {
		return
	}
	ks.mu.Lock()
	defer ks.mu.Unlock()
	for _, ik := range issued {
		ks.keys[ik.ConsumerID] = ik
	}
}

func (ks *KeyStore) save() {
	ks.mu.RLock()
	defer ks.mu.RUnlock()
	ks.doSave()
}

func (ks *KeyStore) doSave() {
	all := make([]*IssuedKey, 0, len(ks.keys))
	for _, ik := range ks.keys {
		all = append(all, ik)
	}
	b, _ := json.MarshalIndent(all, "", "  ")
	os.MkdirAll(filepath.Dir(ks.dataPath), 0755)
	os.WriteFile(ks.dataPath, b, 0600)
}

// IssueKey creates a new signed key for a consumer.
func (ks *KeyStore) IssueKey(consumerID string, quota int64, models []string, expDays int) (string, error) {
	if node == nil || !node.IsInitialized() {
		return "", fmt.Errorf("node identity not initialized")
	}
	if consumerID == "" {
		return "", fmt.Errorf("consumer_id is required")
	}
	if quota <= 0 {
		quota = 15000 // default
	}
	if expDays <= 0 {
		expDays = 30
	}

	// Check contribution points vs frozen quota
	if netMgr != nil {
		netMgr.mu.RLock()
		contribPoints := netMgr.config.ContribPoints
		netMgr.mu.RUnlock()

		frozen := ks.totalFrozenQuota()
		if contribPoints > 0 && (contribPoints - frozen) < quota {
			return "", fmt.Errorf("insufficient contribution points: have %d (frozen %d), need %d", contribPoints, frozen, quota)
		}
	}

	now := time.Now()
	payload := KeyPayload{
		Sub:    consumerID,
		Iss:    netMgr.GetNodeID(),
		Quota:  quota,
		Used:   0,
		Models: models,
		Iat:    now.Unix(),
		Exp:    now.Add(time.Duration(expDays) * 24 * time.Hour).Unix(),
	}

	// Marshal payload to JSON, then base64
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal payload: %w", err)
	}
	payloadB64 := base64.RawURLEncoding.EncodeToString(payloadJSON)

	// Sign the payload with Ed25519 private key
	node.mu.RLock()
	privKey := node.privKey
	node.mu.RUnlock()
	if privKey == nil {
		return "", fmt.Errorf("node private key not available")
	}
	sig := ed25519.Sign(privKey, []byte(payloadB64))
	sigHex := hex.EncodeToString(sig)

	// Build mk_ key
	fullKey := fmt.Sprintf("mk_%s.%s.%s", consumerID, payloadB64, sigHex)

	// Store issued key record
	ik := &IssuedKey{
		ConsumerID: consumerID,
		Key:        fullKey,
		Payload:    payload,
		IssuedAt:   now.Format(time.RFC3339),
		Revoked:    false,
	}

	ks.mu.Lock()
	ks.keys[consumerID] = ik
	ks.mu.Unlock()
	ks.save()

	slog.Info("issued signed key", "consumer_id", consumerID, "quota", quota, "exp_days", expDays)
	return fullKey, nil
}

// RevokeKey marks a key as revoked.
func (ks *KeyStore) RevokeKey(consumerID string) error {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	ik, ok := ks.keys[consumerID]
	if !ok {
		return fmt.Errorf("key not found for consumer: %s", consumerID)
	}
	ik.Revoked = true
	ks.doSave()
	slog.Info("revoked signed key", "consumer_id", consumerID)
	return nil
}

// GetAllKeys returns all issued keys (non-revoked).
func (ks *KeyStore) GetAllKeys() []*IssuedKey {
	ks.mu.RLock()
	defer ks.mu.RUnlock()
	result := make([]*IssuedKey, 0, len(ks.keys))
	for _, ik := range ks.keys {
		result = append(result, ik)
	}
	return result
}

// totalFrozenQuota returns the sum of quotas for all active (non-revoked) keys.
func (ks *KeyStore) totalFrozenQuota() int64 {
	ks.mu.RLock()
	defer ks.mu.RUnlock()
	var total int64
	for _, ik := range ks.keys {
		if !ik.Revoked {
			total += ik.Payload.Quota
		}
	}
	return total
}

// RecordUsage increments the used counter for a consumer key.
func (ks *KeyStore) RecordUsage(consumerID string) {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	if ik, ok := ks.keys[consumerID]; ok && !ik.Revoked {
		ik.Payload.Used++
		// save periodically, not on every request (performance)
	}
}

// SaveAsync saves the key store (called periodically).
func (ks *KeyStore) SaveAsync() {
	ks.save()
}

// ============================================================
// Key Validation
// ============================================================

// ValidateKey parses and validates a mk_ format key.
// Returns the payload if valid, or an error.
func ValidateKey(mkKey string) (*KeyPayload, error) {
	if !strings.HasPrefix(mkKey, "mk_") {
		return nil, fmt.Errorf("not a signed key (missing mk_ prefix)")
	}

	// Strip mk_ prefix
	rest := mkKey[3:]

	// Split into consumer_id.payload_b64.signature_hex
	parts := strings.SplitN(rest, ".", 3)
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid key format: expected mk_{consumer_id}.{payload}.{signature}")
	}

	consumerID := parts[0]
	payloadB64 := parts[1]
	sigHex := parts[2]

	// Decode payload
	payloadJSON, err := base64.RawURLEncoding.DecodeString(payloadB64)
	if err != nil {
		return nil, fmt.Errorf("invalid payload base64: %w", err)
	}

	var payload KeyPayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return nil, fmt.Errorf("invalid payload JSON: %w", err)
	}

	// Verify consumer_id matches
	if payload.Sub != consumerID {
		return nil, fmt.Errorf("consumer_id mismatch: key=%s, payload=%s", consumerID, payload.Sub)
	}

	// Check if revoked by local store
	if keyStore != nil {
		keyStore.mu.RLock()
		if ik, ok := keyStore.keys[consumerID]; ok && ik.Revoked {
			keyStore.mu.RUnlock()
			return nil, fmt.Errorf("key has been revoked")
		}
		keyStore.mu.RUnlock()
	}

	// Check expiration
	now := time.Now().Unix()
	if payload.Exp > 0 && now > payload.Exp {
		return nil, fmt.Errorf("key expired at %d", payload.Exp)
	}

	// Check quota
	if payload.Quota > 0 && payload.Used >= payload.Quota {
		return nil, fmt.Errorf("quota exhausted: used=%d, quota=%d", payload.Used, payload.Quota)
	}

	// Get issuer's public key from route table or known peers
	issuerPubKey := getIssuerPublicKey(payload.Iss)
	if issuerPubKey == nil {
		return nil, fmt.Errorf("cannot find public key for issuer node: %s", payload.Iss)
	}

	// Verify Ed25519 signature
	sigBytes, err := hex.DecodeString(sigHex)
	if err != nil {
		return nil, fmt.Errorf("invalid signature hex: %w", err)
	}

	if !ed25519.Verify(issuerPubKey, []byte(payloadB64), sigBytes) {
		return nil, fmt.Errorf("signature verification failed")
	}

	return &payload, nil
}

// getIssuerPublicKey retrieves the Ed25519 public key for a given NodeID.
// First checks if it's this node, then checks known peers' public keys.
func getIssuerPublicKey(issuerNodeID string) ed25519.PublicKey {
	// Check if it's this node
	if node != nil && node.IsInitialized() {
		selfP2P := DeriveP2PNodeID()
		if issuerNodeID == selfP2P {
			node.mu.RLock()
			pub := node.pubKey
			node.mu.RUnlock()
			return pub
		}
	}

	// Try to find the peer's public key from the network config or federation
	// For Phase 2, we look up from known peers stored in network config
	// In production, this would query the peer's /api/node/pubkey endpoint
	if netMgr != nil {
		netMgr.mu.RLock()
		for _, peer := range netMgr.config.Peers {
			if peer.NodeID == issuerNodeID && len(peer.Addresses) > 0 {
				netMgr.mu.RUnlock()
				// Fetch public key from the peer
				pubKey := fetchPeerPublicKey(peer.Addresses)
				if pubKey != nil {
					return pubKey
				}
				return nil
			}
		}
		netMgr.mu.RUnlock()
	}

	return nil
}

// fetchPeerPublicKey fetches the Ed25519 public key from a peer node.
func fetchPeerPublicKey(addresses []string) ed25519.PublicKey {
	client := &httpClient10
	for _, addr := range addresses {
		addr = strings.TrimRight(addr, "/")
		resp, err := client.Get(addr + "/api/node/pubkey")
		if err != nil {
			continue
		}
		if resp.StatusCode != 200 {
			resp.Body.Close()
			continue
		}
		var result struct {
			PubKeyB64 string `json:"pub_key"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			resp.Body.Close()
			continue
		}
		resp.Body.Close()

		pubBytes, err := base64.StdEncoding.DecodeString(result.PubKeyB64)
		if err != nil || len(pubBytes) != ed25519.PublicKeySize {
			continue
		}
		return ed25519.PublicKey(pubBytes)
	}
	return nil
}

// CheckModelAccess checks if a model is allowed by the key's model list.
func CheckModelAccess(payload *KeyPayload, model string) bool {
	if len(payload.Models) == 0 {
		return true // empty list = all models allowed
	}
	for _, m := range payload.Models {
		if m == model || m == "*" {
			return true
		}
	}
	return false
}

// ============================================================
// API Handlers — Signed Keys
// ============================================================

// POST /api/network/keys/issue (JWT) — issue a new signed key
func handleNetworkKeyIssue(w http.ResponseWriter, r *http.Request) {
	if !netMgr.IsSharedMode() {
		writeError(w, 400, "shared network not active")
		return
	}
	var body struct {
		ConsumerID string   `json:"consumer_id"`
		Quota      int64    `json:"quota"`
		Models     []string `json:"models"`
		ExpDays    int      `json:"exp_days"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, 400, "invalid request body")
		return
	}
	if body.ConsumerID == "" {
		writeError(w, 400, "consumer_id is required")
		return
	}

	key, err := keyStore.IssueKey(body.ConsumerID, body.Quota, body.Models, body.ExpDays)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}

	writeJSON(w, 200, map[string]any{
		"status":      "issued",
		"key":         key,
		"consumer_id": body.ConsumerID,
		"quota":       body.Quota,
		"models":      body.Models,
	})
}

// GET /api/network/keys (JWT) — list all issued keys
func handleNetworkKeyList(w http.ResponseWriter, r *http.Request) {
	if !netMgr.IsSharedMode() {
		writeJSON(w, 200, map[string]any{"keys": []any{}, "message": "shared network not active"})
		return
	}
	keys := keyStore.GetAllKeys()
	writeJSON(w, 200, map[string]any{"keys": keys, "count": len(keys)})
}

// DELETE /api/network/keys/{consumer_id} (JWT) — revoke a key
func handleNetworkKeyRevoke(w http.ResponseWriter, r *http.Request) {
	consumerID := r.PathValue("consumer_id")
	if consumerID == "" {
		writeError(w, 400, "consumer_id is required")
		return
	}
	if err := keyStore.RevokeKey(consumerID); err != nil {
		writeError(w, 404, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"status": "revoked", "consumer_id": consumerID})
}

// POST /api/network/keys/validate (no auth) — validate a signed key
func handleNetworkKeyValidate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Key   string `json:"key"`
		Model string `json:"model"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, 400, "invalid request body")
		return
	}
	if body.Key == "" {
		writeError(w, 400, "key is required")
		return
	}

	payload, err := ValidateKey(body.Key)
	if err != nil {
		writeJSON(w, 200, map[string]any{"valid": false, "error": err.Error()})
		return
	}

	// Check model access if model is specified
	if body.Model != "" && !CheckModelAccess(payload, body.Model) {
		writeJSON(w, 200, map[string]any{
			"valid":  false,
			"error":  fmt.Sprintf("model '%s' not allowed by this key", body.Model),
			"allowed": payload.Models,
		})
		return
	}

	writeJSON(w, 200, map[string]any{
		"valid":       true,
		"consumer_id": payload.Sub,
		"issuer":      payload.Iss,
		"quota":       payload.Quota,
		"used":        payload.Used,
		"remaining":   payload.Quota - payload.Used,
		"models":      payload.Models,
		"expires":     payload.Exp,
	})
}

// GET /api/network/contributions (JWT) — view contribution records
func handleNetworkContributions(w http.ResponseWriter, r *http.Request) {
	if !netMgr.IsSharedMode() {
		writeJSON(w, 200, map[string]any{"records": []any{}, "message": "shared network not active"})
		return
	}

	netMgr.mu.RLock()
	status := map[string]any{
		"contrib_points":     netMgr.config.ContribPoints,
		"frozen_quota":       keyStore.totalFrozenQuota(),
		"requests_relayed":   netMgr.config.Stats.RequestsRelayed,
		"relay_success":      netMgr.config.Stats.RelaySuccess,
		"relay_failed":       netMgr.config.Stats.RelayFailed,
		"records":            netMgr.config.ContribRecords,
		"issued_keys_count":  len(keyStore.keys),
	}
	netMgr.mu.RUnlock()

	writeJSON(w, 200, status)
}
