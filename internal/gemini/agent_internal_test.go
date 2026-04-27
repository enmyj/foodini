package gemini

import (
	"strings"
	"testing"
	"time"
)

// formatAgentContext should emit meal sections in canonical order (breakfast,
// snack, lunch, dinner, supplements) regardless of map iteration order, so the
// prompt is stable across calls for identical state.
func TestFormatAgentContext_StableMealOrder(t *testing.T) {
	ac := AgentContext{
		Date: "2026-04-26",
		TodayByMeal: map[string][]Entry{
			"dinner":    {{Description: "salmon", Calories: 400}},
			"breakfast": {{Description: "oats", Calories: 300}},
			"lunch":     {{Description: "salad", Calories: 350}},
		},
	}
	var first string
	for i := range 20 {
		out := formatAgentContext(ac)
		if i == 0 {
			first = out
			continue
		}
		if out != first {
			t.Fatalf("formatAgentContext nondeterministic:\nfirst:\n%s\n\nlater:\n%s", first, out)
		}
	}
	bIdx := strings.Index(first, "breakfast:")
	lIdx := strings.Index(first, "lunch:")
	dIdx := strings.Index(first, "dinner:")
	if !(bIdx > 0 && bIdx < lIdx && lIdx < dIdx) {
		t.Errorf("expected breakfast < lunch < dinner ordering; got indexes b=%d l=%d d=%d", bIdx, lIdx, dIdx)
	}
}

// Sessions must evict idle entries past TTL so the map can't grow without
// bound across many dates.
func TestSessionStore_EvictsExpired(t *testing.T) {
	st := &agentSessionStore{sessions: map[string]*AgentSession{
		"a|2026-01-01": {lastUsed: time.Now().Add(-2 * agentSessionTTL)},
		"a|2026-04-26": {lastUsed: time.Now()},
	}}
	st.evictExpiredLocked(time.Now())
	if _, ok := st.sessions["a|2026-01-01"]; ok {
		t.Error("expected stale session to be evicted")
	}
	if _, ok := st.sessions["a|2026-04-26"]; !ok {
		t.Error("expected fresh session to be retained")
	}
}
