package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// ============================================================
// Credits Manager
// ============================================================

type CreditsManager struct {
	mu           sync.RWMutex
	balance      int
	transactions []CreditTransaction
	dataDir      string
}

var credits *CreditsManager

// Credit rules
const (
	creditEarnPer1KTokens  = 1   // +1 per 1000 tokens served via relay
	creditSpendPer1KTokens = 1   // -1 per 1000 tokens used via relay
	creditSpendPerMessage  = 5   // -5 per P2P message sent
	creditInviteBonus      = 50  // +50 when invited node is approved
	creditDailySpendCap    = 1000
)

func initCredits(dataDir string) {
	credits = &CreditsManager{
		balance:      0,
		transactions: []CreditTransaction{},
		dataDir:      dataDir,
	}
	credits.load()
	slog.Info("credits manager initialized", "balance", credits.balance)
}

// ============================================================
// Balance & transactions
// ============================================================

func (c *CreditsManager) GetBalance() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.balance
}

func (c *CreditsManager) AddCredits(amount int, reason string, fromNode string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	tx := CreditTransaction{
		ID:        generateTxID(),
		FromNode:  fromNode,
		ToNode:    "",
		Amount:    amount,
		Reason:    reason,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	c.balance += amount
	c.transactions = append(c.transactions, tx)

	// Keep only last 10000 transactions in memory
	if len(c.transactions) > 10000 {
		c.transactions = c.transactions[len(c.transactions)-10000:]
	}

	c.save()
	slog.Info("credits added", "amount", amount, "reason", reason, "from", fromNode, "balance", c.balance)
}

func (c *CreditsManager) SpendCredits(amount int, reason string, toNode string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check daily spending cap
	todaySpent := c.todaySpending()
	if todaySpent+amount > creditDailySpendCap {
		slog.Warn("daily credit cap exceeded", "today_spent", todaySpent, "requested", amount, "cap", creditDailySpendCap)
		return false
	}

	if c.balance < amount {
		slog.Warn("insufficient credits", "balance", c.balance, "requested", amount)
		return false
	}

	tx := CreditTransaction{
		ID:        generateTxID(),
		FromNode:  "",
		ToNode:    toNode,
		Amount:    -amount,
		Reason:    reason,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	c.balance -= amount
	c.transactions = append(c.transactions, tx)

	if len(c.transactions) > 10000 {
		c.transactions = c.transactions[len(c.transactions)-10000:]
	}

	c.save()
	slog.Info("credits spent", "amount", amount, "reason", reason, "to", toNode, "balance", c.balance)
	return true
}

func (c *CreditsManager) GetTransactions(limit int) []CreditTransaction {
	c.mu.RLock()
	defer c.mu.RUnlock()

	n := len(c.transactions)
	if limit <= 0 || limit > n {
		limit = n
	}

	// Return most recent first
	result := make([]CreditTransaction, limit)
	for i := 0; i < limit; i++ {
		result[i] = c.transactions[n-1-i]
	}
	return result
}

// todaySpending returns total credits spent today. Must be called under lock.
func (c *CreditsManager) todaySpending() int {
	today := time.Now().UTC().Format("2006-01-02")
	total := 0
	for i := len(c.transactions) - 1; i >= 0; i-- {
		tx := c.transactions[i]
		if tx.Amount >= 0 {
			continue // skip earnings
		}
		if len(tx.Timestamp) < 10 || tx.Timestamp[:10] != today {
			continue
		}
		total += -tx.Amount
	}
	return total
}

// ============================================================
// Transaction ID generation
// ============================================================

func generateTxID() string {
	b := make([]byte, 8)
	_, err := rand.Read(b)
	if err != nil {
		// Fallback to timestamp-based
		return fmt.Sprintf("tx_%d", time.Now().UnixNano())
	}
	return "tx_" + hex.EncodeToString(b)
}

// ============================================================
// Persistence
// ============================================================

func (c *CreditsManager) save() {
	path := filepath.Join(c.dataDir, "credits.json")

	data := struct {
		Balance      int                 `json:"balance"`
		Transactions []CreditTransaction `json:"transactions"`
	}{
		Balance:      c.balance,
		Transactions: c.transactions,
	}

	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		slog.Error("failed to marshal credits data", "error", err)
		return
	}
	if err := os.WriteFile(path, raw, 0644); err != nil {
		slog.Error("failed to write credits file", "error", err)
	}
}

func (c *CreditsManager) load() {
	path := filepath.Join(c.dataDir, "credits.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Error("failed to read credits file", "error", err)
		}
		return
	}

	var data struct {
		Balance      int                 `json:"balance"`
		Transactions []CreditTransaction `json:"transactions"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		slog.Error("failed to unmarshal credits data", "error", err)
		return
	}

	c.balance = data.Balance
	if data.Transactions != nil {
		c.transactions = data.Transactions
	}
}

// ============================================================
// HTTP Handlers
// ============================================================

// handleGetCredits returns current balance.
// GET /federation/credits
func handleGetCredits(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, 405, "method not allowed")
		return
	}

	writeJSON(w, 200, map[string]any{
		"balance":       credits.GetBalance(),
		"daily_cap":     creditDailySpendCap,
		"earned_per_1k": creditEarnPer1KTokens,
		"spent_per_1k":  creditSpendPer1KTokens,
		"spent_per_msg": creditSpendPerMessage,
		"invite_bonus":  creditInviteBonus,
	})
}

// handleGetCreditHistory returns recent transactions.
// GET /federation/credits/history?limit=50
func handleGetCreditHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, 405, "method not allowed")
		return
	}

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 500 {
		limit = 500
	}

	txs := credits.GetTransactions(limit)
	writeJSON(w, 200, map[string]any{
		"transactions": txs,
		"count":        len(txs),
		"balance":      credits.GetBalance(),
	})
}
