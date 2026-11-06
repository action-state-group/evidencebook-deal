package deal

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// reconcileOfListing is the deal's `reconcile` member in `deal list --json`.
func reconcileOfListing(t *testing.T, id string, extra ...string) any {
	t.Helper()
	text, err := invoke(t, "", append([]string{"--profile", "deal", "deal", "list", "--json"}, extra...)...)
	require.NoError(t, err, text)
	var list []map[string]any
	if extra == nil {
		require.NoError(t, json.Unmarshal([]byte(text), &list), text)
	} else {
		var ranged struct {
			Deals []map[string]any `json:"deals"`
		}
		require.NoError(t, json.Unmarshal([]byte(text), &ranged), text)
		list = ranged.Deals
	}
	for _, l := range list {
		if l["deal_id"] == id {
			require.Contains(t, l, "reconcile")
			return l["reconcile"]
		}
	}
	require.Failf(t, "deal not listed", "%s in %s", id, text)
	return nil
}

// A deal with no sealed Result states nothing: null, never false.
func TestListReconcileIsNullBeforeAnyResult(t *testing.T) {
	pair := closedPair(t, received, received)
	assert.Nil(t, reconcileOfListing(t, pair.id))
}

// Matching halves: the counterparty's half is held and nothing conflicts.
func TestListReconcileHeldAndNotConflicting(t *testing.T) {
	pair := closedPair(t, received, received)
	_, _, _, err := runTermsResult(t, pair.id, pair.response, "--openings", pair.sellerFile)
	require.NoError(t, err)
	got, _ := reconcileOfListing(t, pair.id).(map[string]any)
	assert.Equal(t, true, got["counterparty_half_held"])
	assert.Equal(t, false, got["halves_conflict"])
	_, perr := time.Parse(time.RFC3339, got["at"].(string))
	assert.NoError(t, perr)
	assert.NotNil(t, reconcileOfListing(t, pair.id, "--since", "2000-01-01"), "a ranged listing carries it too")
	assert.NotNil(t, reconcileOfListing(t, pair.id, "--until", "2100-01-01"))
}

// Halves that disagree on the amount: held, and conflicting.
func TestListReconcileHeldAndConflicting(t *testing.T) {
	pair := closedPair(t, received, `{"status":"received","delivered":{"price_minor":185000}}`)
	_, _, _, err := runTermsResult(t, pair.id, pair.response, "--openings", pair.sellerFile)
	require.NoError(t, err)
	got, _ := reconcileOfListing(t, pair.id).(map[string]any)
	assert.Equal(t, true, got["counterparty_half_held"])
	assert.Equal(t, true, got["halves_conflict"])
}

// A Result made without the counterparty's openings cites only the holder's
// half: the counterparty's is not held.
func TestListReconcileOwnHalfOnly(t *testing.T) {
	pair := closedPair(t, received, received)
	_, _, _, err := runTermsResult(t, pair.id, pair.response)
	require.NoError(t, err)
	got, _ := reconcileOfListing(t, pair.id).(map[string]any)
	assert.Equal(t, false, got["counterparty_half_held"])
	assert.Equal(t, false, got["halves_conflict"])
}
