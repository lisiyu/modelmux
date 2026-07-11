package main

import (
	"sync"
	"testing"
	"time"
)

// ============================================================
// §3.2.3 Public Global Key Four-Layer Quota Tests
// ============================================================

func newTestPublicKeyQuota() *PublicKeyQuota {
	return &PublicKeyQuota{
		GlobalDailyLimit:  10000,
		IPDailyLimit:      2000,
		HourlyWindowLimit: 500,
		ModelLimits: map[string]int64{
			"gpt-4o":        1000,
			"deepseek-chat": 1000,
		},
		ipUsage:       make(map[string]*IPUsageTracker),
		hourlyUsage:   make(map[string]int64),
		modelUsage:    make(map[string]int64),
		lastDailyReset:  time.Now(),
		lastHourlyReset: time.Now(),
	}
}

func TestCheckQuota_Allowed(t *testing.T) {
	q := newTestPublicKeyQuota()

	allowed, reason, remaining := q.CheckQuota("1.2.3.4", "gpt-4o", 100)
	if !allowed {
		t.Errorf("small request should be allowed, got: %s", reason)
	}
	if remaining <= 0 {
		t.Errorf("remaining should be > 0, got %d", remaining)
	}
}

func TestCheckQuota_GlobalExhausted(t *testing.T) {
	q := newTestPublicKeyQuota()
	q.globalUsedToday = q.GlobalDailyLimit // exhaust global

	allowed, reason, remaining := q.CheckQuota("1.2.3.4", "gpt-4o", 100)
	if allowed {
		t.Error("should be rejected when global quota exhausted")
	}
	if remaining != 0 {
		t.Errorf("remaining should be 0 when global exhausted, got %d", remaining)
	}
	if reason == "" {
		t.Error("should have a rejection reason")
	}
}

func TestCheckQuota_IPExhausted(t *testing.T) {
	q := newTestPublicKeyQuota()
	q.ipUsage["1.2.3.4"] = &IPUsageTracker{
		DailyUsed: q.IPDailyLimit, // exhaust IP
		LastReset: time.Now(),
	}

	allowed, reason, _ := q.CheckQuota("1.2.3.4", "gpt-4o", 100)
	if allowed {
		t.Error("should be rejected when IP quota exhausted")
	}
	if reason == "" {
		t.Error("should have a rejection reason for IP exhaustion")
	}
}

func TestCheckQuota_HourlyExhausted(t *testing.T) {
	q := newTestPublicKeyQuota()
	hourKey := time.Now().Format("2006-01-02-15")
	q.hourlyUsage[hourKey] = q.HourlyWindowLimit // exhaust hourly

	allowed, reason, _ := q.CheckQuota("1.2.3.4", "gpt-4o", 100)
	if allowed {
		t.Error("should be rejected when hourly quota exhausted")
	}
	if reason == "" {
		t.Error("should have a rejection reason for hourly exhaustion")
	}
}

func TestCheckQuota_ModelExhausted(t *testing.T) {
	q := newTestPublicKeyQuota()
	q.modelUsage["gpt-4o"] = q.ModelLimits["gpt-4o"] // exhaust model

	allowed, reason, _ := q.CheckQuota("1.2.3.4", "gpt-4o", 100)
	if allowed {
		t.Error("should be rejected when model quota exhausted")
	}
	if reason == "" {
		t.Error("should have a rejection reason for model exhaustion")
	}
}

func TestCheckQuota_UnknownModel(t *testing.T) {
	q := newTestPublicKeyQuota()

	// Unknown model (no per-model limit) should still pass other layers
	allowed, reason, _ := q.CheckQuota("1.2.3.4", "claude-3-opus", 100)
	if !allowed {
		t.Errorf("unknown model should be allowed (no model-specific limit), got: %s", reason)
	}
}

func TestRecordUsage(t *testing.T) {
	q := newTestPublicKeyQuota()

	q.RecordUsage("1.2.3.4", "gpt-4o", 500)

	// Verify global
	if q.globalUsedToday != 500 {
		t.Errorf("globalUsedToday = %d, want 500", q.globalUsedToday)
	}

	// Verify IP
	if q.ipUsage["1.2.3.4"].DailyUsed != 500 {
		t.Errorf("IP daily used = %d, want 500", q.ipUsage["1.2.3.4"].DailyUsed)
	}

	// Verify model
	if q.modelUsage["gpt-4o"] != 500 {
		t.Errorf("model usage = %d, want 500", q.modelUsage["gpt-4o"])
	}
}

func TestRecordUsage_Multiple(t *testing.T) {
	q := newTestPublicKeyQuota()

	q.RecordUsage("1.2.3.4", "gpt-4o", 500)
	q.RecordUsage("1.2.3.4", "gpt-4o", 300)
	q.RecordUsage("5.6.7.8", "deepseek-chat", 200)

	// Verify global total
	if q.globalUsedToday != 1000 {
		t.Errorf("globalUsedToday = %d, want 1000", q.globalUsedToday)
	}

	// Verify per-IP
	if q.ipUsage["1.2.3.4"].DailyUsed != 800 {
		t.Errorf("IP 1.2.3.4 daily used = %d, want 800", q.ipUsage["1.2.3.4"].DailyUsed)
	}
	if q.ipUsage["5.6.7.8"].DailyUsed != 200 {
		t.Errorf("IP 5.6.7.8 daily used = %d, want 200", q.ipUsage["5.6.7.8"].DailyUsed)
	}

	// Verify per-model
	if q.modelUsage["gpt-4o"] != 800 {
		t.Errorf("gpt-4o usage = %d, want 800", q.modelUsage["gpt-4o"])
	}
	if q.modelUsage["deepseek-chat"] != 200 {
		t.Errorf("deepseek-chat usage = %d, want 200", q.modelUsage["deepseek-chat"])
	}
}

func TestCheckQuota_SequentialUsage(t *testing.T) {
	// Use a quota where hourly limit is high enough to not interfere
	q := &PublicKeyQuota{
		GlobalDailyLimit:  10000,
		IPDailyLimit:      2000,
		HourlyWindowLimit: 5000, // high enough to not be the bottleneck
		ModelLimits: map[string]int64{
			"gpt-4o": 5000, // high enough to not be the bottleneck
		},
		ipUsage:        make(map[string]*IPUsageTracker),
		hourlyUsage:    make(map[string]int64),
		modelUsage:     make(map[string]int64),
		lastDailyReset: time.Now(),
		lastHourlyReset: time.Now(),
	}

	// Use 100 tokens at a time, should succeed until IP daily limit (2000) reached
	for i := 0; i < 20; i++ {
		allowed, _, _ := q.CheckQuota("1.2.3.4", "gpt-4o", 100)
		if !allowed {
			t.Fatalf("request %d should be allowed", i+1)
		}
		q.RecordUsage("1.2.3.4", "gpt-4o", 100)
	}

	// After 20 × 100 = 2000 tokens, IP daily limit (2000) should be hit
	allowed, reason, _ := q.CheckQuota("1.2.3.4", "gpt-4o", 100)
	if allowed {
		t.Error("should be rejected after IP daily limit reached")
	}
	t.Logf("rejection reason: %s", reason)
}

func TestGetQuotaStatus(t *testing.T) {
	q := newTestPublicKeyQuota()
	q.RecordUsage("1.2.3.4", "gpt-4o", 500)

	status := q.GetQuotaStatus()

	if status["enabled"] != true {
		t.Error("quota should be enabled")
	}
	if status["global_daily_limit"] != int64(10000) {
		t.Error("global_daily_limit mismatch")
	}
	if status["global_used_today"] != int64(500) {
		t.Errorf("global_used_today = %v, want 500", status["global_used_today"])
	}
}

func TestResetIfNeededLocked(t *testing.T) {
	q := newTestPublicKeyQuota()
	q.RecordUsage("1.2.3.4", "gpt-4o", 500)

	// Simulate day change
	q.mu.Lock()
	q.lastDailyReset = time.Now().Add(-25 * time.Hour)
	q.resetIfNeededLocked()
	q.mu.Unlock()

	if q.globalUsedToday != 0 {
		t.Errorf("globalUsedToday should be reset to 0, got %d", q.globalUsedToday)
	}
	if len(q.ipUsage) != 0 {
		t.Errorf("ipUsage should be cleared after daily reset, got %d entries", len(q.ipUsage))
	}
}

func TestGlobalPoolManager_CheckPublicKeyQuota(t *testing.T) {
	// Initialize public quota
	publicQuota = newTestPublicKeyQuota()
	defer func() { publicQuota = nil }()

	gm := NewGlobalPoolManager()

	allowed, reason, _ := gm.CheckPublicKeyQuota("1.2.3.4", "gpt-4o", 100)
	if !allowed {
		t.Errorf("should be allowed, got: %s", reason)
	}
}

func TestGlobalPoolManager_CheckPublicKeyQuota_NilQuota(t *testing.T) {
	publicQuota = nil
	gm := NewGlobalPoolManager()

	// Should allow when quota system not initialized
	allowed, _, _ := gm.CheckPublicKeyQuota("1.2.3.4", "gpt-4o", 100)
	if !allowed {
		t.Error("should allow when quota system not initialized")
	}
}

// TestConcurrentQuotaRace verifies that concurrent requests cannot bypass quota limits.
// This tests the TOCTOU fix: ReserveQuota atomically checks and reserves in one operation.
func TestConcurrentQuotaRace(t *testing.T) {
	q := &PublicKeyQuota{
		GlobalDailyLimit:  10000,
		IPDailyLimit:      500,
		HourlyWindowLimit: 5000,
		ModelLimits: map[string]int64{
			"gpt-4o": 500,
		},
		ipUsage:        make(map[string]*IPUsageTracker),
		hourlyUsage:    make(map[string]int64),
		modelUsage:     make(map[string]int64),
		lastDailyReset: time.Now(),
		lastHourlyReset: time.Now(),
	}

	ip := "10.0.0.99"
	model := "gpt-4o"
	const goroutines = 100
	const requestTokens int64 = 10

	var reserved int64
	var mu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			ok, _, _ := q.ReserveQuota(ip, model, requestTokens)
			if ok {
				mu.Lock()
				reserved += requestTokens
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	// The IP daily limit is 500, so at most 500/10 = 50 requests can succeed
	if reserved > q.IPDailyLimit {
		t.Errorf("reserved %d tokens exceeds IP daily limit %d — TOCTOU race detected!", reserved, q.IPDailyLimit)
	}
	if reserved > q.ModelLimits[model] {
		t.Errorf("reserved %d tokens exceeds model limit %d — TOCTOU race detected!", reserved, q.ModelLimits[model])
	}

	t.Logf("reserved %d tokens out of %d goroutine requests (IP limit=%d, model limit=%d)",
		reserved, goroutines*requestTokens, q.IPDailyLimit, q.ModelLimits[model])

	// Verify internal counters are consistent
	q.mu.RLock()
	defer q.mu.RUnlock()
	if q.globalUsedToday != reserved {
		t.Errorf("globalUsedToday=%d, expected %d", q.globalUsedToday, reserved)
	}
	if q.ipUsage[ip].DailyUsed != reserved {
		t.Errorf("IP daily used=%d, expected %d", q.ipUsage[ip].DailyUsed, reserved)
	}
	if q.modelUsage[model] != reserved {
		t.Errorf("model usage=%d, expected %d", q.modelUsage[model], reserved)
	}
}

// TestReserveAndAdjustQuota verifies the reserve + adjust flow.
func TestReserveAndAdjustQuota(t *testing.T) {
	q := &PublicKeyQuota{
		GlobalDailyLimit:  10000,
		IPDailyLimit:      5000,
		HourlyWindowLimit: 5000,
		ModelLimits:       map[string]int64{"gpt-4o": 5000},
		ipUsage:           make(map[string]*IPUsageTracker),
		hourlyUsage:       make(map[string]int64),
		modelUsage:        make(map[string]int64),
		lastDailyReset:    time.Now(),
		lastHourlyReset:   time.Now(),
	}

	// Reserve 500 tokens
	ok, _, _ := q.ReserveQuota("1.2.3.4", "gpt-4o", 500)
	if !ok {
		t.Fatal("reserve should succeed")
	}
	if q.globalUsedToday != 500 {
		t.Errorf("after reserve: globalUsedToday=%d, want 500", q.globalUsedToday)
	}

	// Adjust: actual usage was 300 (refunded 200)
	q.AdjustQuota("1.2.3.4", "gpt-4o", 500, 300)
	if q.globalUsedToday != 300 {
		t.Errorf("after adjust down: globalUsedToday=%d, want 300", q.globalUsedToday)
	}

	// Reserve another 100
	ok, _, _ = q.ReserveQuota("1.2.3.4", "gpt-4o", 100)
	if !ok {
		t.Fatal("second reserve should succeed")
	}

	// Adjust: actual was 150 (charged 50 extra)
	q.AdjustQuota("1.2.3.4", "gpt-4o", 100, 150)
	if q.globalUsedToday != 450 {
		t.Errorf("after adjust up: globalUsedToday=%d, want 450", q.globalUsedToday)
	}

	// Adjust with zero diff (no-op)
	q.AdjustQuota("1.2.3.4", "gpt-4o", 0, 0)
	if q.globalUsedToday != 450 {
		t.Errorf("after zero adjust: globalUsedToday=%d, want 450", q.globalUsedToday)
	}
}
