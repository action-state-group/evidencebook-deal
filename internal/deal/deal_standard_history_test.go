package deal

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/action-state-group/agent-action-capsule/go/standardhistory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func readBundle(t *testing.T, path string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	var b map[string]any
	require.NoError(t, json.Unmarshal(raw, &b))
	return b
}

func shBlock(t *testing.T, b map[string]any) map[string]any {
	t.Helper()
	block, ok := b["extensions"].(map[string]any)[shExtension].(map[string]any)
	require.True(t, ok, "the bundle carries the x-standard-history/v0 block")
	return block
}

// No declaration, no card: the holder kind is never defaulted, and a card
// needs a checkpoint to be as of.
func TestNoCardWithoutADeclarationOrATick(t *testing.T) {
	dealFixture(t)
	out := filepath.Join(t.TempDir(), "card.json")
	_, err := invoke(t, "", "--profile", "deal", "deal", "standard-history", "card", "--out", out)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "declare")

	_, err = invoke(t, "", "--profile", "deal", "deal", "standard-history", "declare")
	require.Error(t, err, "the kind is required")
	dealRun(t, "standard-history", "declare", "--holder-kind", "node")
	_, err = invoke(t, "", "--profile", "deal", "deal", "standard-history", "card", "--out", out)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "deal tick")
}

// The answer default follows the holder kind, and a flag overrides it.
func TestTheAnswerDefaultFollowsTheHolderKind(t *testing.T) {
	dealFixture(t)
	node := dealRun(t, "standard-history", "declare", "--holder-kind", "node")
	assert.Equal(t, true, node["answers"])
	assert.Contains(t, node["notice"], "--no-answer")
	assert.Equal(t, false, dealRun(t, "standard-history", "declare", "--holder-kind", "node", "--no-answer")["answers"])
	person := dealRun(t, "standard-history", "declare", "--holder-kind", "person")
	assert.Equal(t, false, person["answers"])
	assert.NotContains(t, person, "notice")
	assert.Equal(t, true, dealRun(t, "standard-history", "declare", "--holder-kind", "person", "--answer")["answers"])
}

// The policy is its own file: the default is the last 12 months, its digest
// is the SHA-256 of the file's octets, and a change moves the digest.
func TestThePolicyDefaultAndDigest(t *testing.T) {
	dealFixture(t)
	first := dealRun(t, "standard-history", "policy")
	assert.Equal(t, map[string]any{"months": float64(12)}, first["window"])
	digest := first["policy_digest"].(string)
	assert.Len(t, digest, 64)
	set := dealRun(t, "standard-history", "policy", "--months", "3")
	assert.Equal(t, map[string]any{"months": float64(3)}, set["window"])
	assert.NotEqual(t, digest, set["policy_digest"])
	assert.Equal(t, map[string]any{"all": true}, dealRun(t, "standard-history", "policy", "--all")["window"])
	_, err := invoke(t, "", "--profile", "deal", "deal", "standard-history", "policy", "--months", "3", "--all")
	require.Error(t, err)
}

// A deal under the anchor is counted, with the amount its check rests on; the
// shared copy carries the card, policy and anchor but never the deal entries.
func TestTheCardCountsTheDealsUnderTheAnchor(t *testing.T) {
	dealFixture(t)
	now := clockAt(t, time.Date(2026, 10, 4, 18, 24, 0, 0, time.UTC))
	id := retailDeal(t)
	_ = id
	*now = now.Add(2 * time.Minute)
	require.Equal(t, "ticked", dealRun(t, "tick")["state"])
	dealRun(t, "standard-history", "declare", "--holder-kind", "node")

	dir := t.TempDir()
	own := filepath.Join(dir, "own.json")
	dealRun(t, "standard-history", "card", "--out", own)
	shared := filepath.Join(dir, "shared.json")
	dealRun(t, "standard-history", "card", "--out", shared, "--shared")

	ownBlock := shBlock(t, readBundle(t, own))
	deals, ok := ownBlock["deals"].([]any)
	require.True(t, ok, "the own copy holds the deal entries")
	require.Len(t, deals, 1)
	entry := deals[0].(map[string]any)
	assert.Equal(t, id, entry["deal"])
	assert.Equal(t, "no_record", entry["happened"])
	assert.Equal(t, "undecided", entry["outcome"])
	assert.Contains(t, entry, "amount")

	sharedBlock := shBlock(t, readBundle(t, shared))
	assert.NotContains(t, sharedBlock, "deals", "no deal entry leaves in the shared copy")
	assert.Equal(t, ownBlock["policy"], sharedBlock["policy"])
	assert.Equal(t, ownBlock["as_of"], sharedBlock["as_of"])
	assert.Equal(t, map[string]any{"months": float64(12)}, ownBlock["policy"].(map[string]any)["window"])
	assert.Equal(t, standardhistory.Derivation, "standard_history/1")
}
