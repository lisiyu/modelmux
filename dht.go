package main

import (
	"crypto/sha256"
	"encoding/binary"
	"log/slog"
	"sort"
	"sync"
)

// DHTTable implements a simplified DHT-style routing table for efficient peer discovery.
// Combined with the existing gossip protocol (Phase 1/2), this provides Phase 3 hybrid discovery.
//
// The DHT ring maps node IDs to positions on a 16-bit hash ring. Each node maintains
// awareness of its neighbors and can efficiently route messages to any node by
// forwarding through intermediate nodes (O(log N) hops).
type DHTTable struct {
	mu       sync.RWMutex
	ring     []dhtEntry    // sorted by hash position
	localPos uint16        // this node's position on the ring
	localID  string        // this node's ID
}

type dhtEntry struct {
	NodeID   string
	Hash     uint16
	Endpoint string
}

var dhtTable *DHTTable

// initDHT initializes the DHT routing table from the current federation state.
func initDHT() {
	if node == nil || !node.IsInitialized() {
		return
	}
	dhtTable = &DHTTable{
		localID:  node.NodeID(),
		localPos: hashToRing(node.NodeID()),
	}
	dhtTable.rebuildFromFederation()
	slog.Info("DHT routing table initialized", "node_id", dhtTable.localID, "ring_pos", dhtTable.localPos)
}

// hashToRing maps a node ID to a position on the 16-bit ring.
func hashToRing(nodeID string) uint16 {
	h := sha256.Sum256([]byte(nodeID))
	return binary.BigEndian.Uint16(h[:2])
}

// rebuildFromFederation populates the ring from the current trust pool.
func (d *DHTTable) rebuildFromFederation() {
	if fed == nil {
		return
	}

	pool := fed.GetTrustPool()
	d.mu.Lock()
	defer d.mu.Unlock()

	d.ring = make([]dhtEntry, 0, len(pool.Nodes))
	for _, n := range pool.Nodes {
		if n.Status != "active" || n.Endpoint == "" {
			continue
		}
		d.ring = append(d.ring, dhtEntry{
			NodeID:   n.NodeID,
			Hash:     hashToRing(n.NodeID),
			Endpoint: n.Endpoint,
		})
	}

	sort.Slice(d.ring, func(i, j int) bool {
		return d.ring[i].Hash < d.ring[j].Hash
	})
}

// FindClosest returns the N closest nodes to a given target on the ring.
// This enables efficient iterative lookups in the DHT.
func (d *DHTTable) FindClosest(targetID string, count int) []dhtEntry {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if len(d.ring) == 0 {
		return nil
	}

	targetPos := hashToRing(targetID)

	// Build a list of distances
	type nodeDist struct {
		entry dhtEntry
		dist  uint16
	}
	distances := make([]nodeDist, 0, len(d.ring))
	for _, e := range d.ring {
		if e.NodeID == d.localID {
			continue
		}
		dist := ringDistance(targetPos, e.Hash)
		distances = append(distances, nodeDist{entry: e, dist: dist})
	}

	sort.Slice(distances, func(i, j int) bool {
		return distances[i].dist < distances[j].dist
	})

	result := make([]dhtEntry, 0, count)
	for i := 0; i < len(distances) && i < count; i++ {
		result = append(result, distances[i].entry)
	}
	return result
}

// GetSuccessors returns the N successor nodes on the ring (clockwise from local).
func (d *DHTTable) GetSuccessors(n int) []dhtEntry {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if len(d.ring) == 0 {
		return nil
	}

	// Find our position in the sorted ring
	idx := d.findLocalIndex()
	result := make([]dhtEntry, 0, n)
	ringLen := len(d.ring)

	for i := 1; i <= n && i < ringLen; i++ {
		pos := (idx + i) % ringLen
		if d.ring[pos].NodeID != d.localID {
			result = append(result, d.ring[pos])
		}
	}
	return result
}

// GetPredecessors returns the N predecessor nodes (counter-clockwise from local).
func (d *DHTTable) GetPredecessors(n int) []dhtEntry {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if len(d.ring) == 0 {
		return nil
	}

	idx := d.findLocalIndex()
	result := make([]dhtEntry, 0, n)
	ringLen := len(d.ring)

	for i := 1; i <= n && i < ringLen; i++ {
		pos := (idx - i + ringLen) % ringLen
		if d.ring[pos].NodeID != d.localID {
			result = append(result, d.ring[pos])
		}
	}
	return result
}

// findLocalIndex returns the index of the local node in the sorted ring.
// Must be called with read lock held.
func (d *DHTTable) findLocalIndex() int {
	for i, e := range d.ring {
		if e.NodeID == d.localID {
			return i
		}
	}
	// If not in ring, find insertion point
	pos := sort.Search(len(d.ring), func(i int) bool {
		return d.ring[i].Hash >= d.localPos
	})
	return pos % len(d.ring)
}

// ringDistance calculates the clockwise distance between two positions on the ring.
func ringDistance(from, to uint16) uint16 {
	if to >= from {
		return to - from
	}
	return 65535 - from + to + 1
}

// GetDHTStats returns DHT routing table statistics.
func GetDHTStats() map[string]any {
	if dhtTable == nil {
		return map[string]any{"enabled": false}
	}

	dhtTable.mu.RLock()
	defer dhtTable.mu.RUnlock()

	successors := dhtTable.GetSuccessors(3)
	predecessors := dhtTable.GetPredecessors(3)

	succIDs := make([]string, 0, len(successors))
	for _, s := range successors {
		succIDs = append(succIDs, s.NodeID)
	}
	predIDs := make([]string, 0, len(predecessors))
	for _, p := range predecessors {
		predIDs = append(predIDs, p.NodeID)
	}

	return map[string]any{
		"enabled":      true,
		"ring_size":    len(dhtTable.ring),
		"local_pos":    dhtTable.localPos,
		"successors":   succIDs,
		"predecessors": predIDs,
	}
}

// RefreshDHT rebuilds the DHT routing table from current federation state.
// Called periodically or when the trust pool changes.
func RefreshDHT() {
	if dhtTable != nil {
		dhtTable.rebuildFromFederation()
	}
}
