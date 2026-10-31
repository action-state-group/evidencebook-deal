package deal

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/action-state-group/agent-action-capsule/go/evidencerequest"
	"github.com/action-state-group/agent-action-capsule/go/standardhistory"
	"github.com/action-state-group/evidencebook"
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

// cardRequest is a standard_history/1 request over full_history in
// evidence-interop's own wire, freshness a log size of 1.
func cardRequest(t *testing.T, subject evidencerequest.Subject) string {
	t.Helper()
	size := uint64(1)
	octets, err := evidencerequest.EncodeRequest(evidencerequest.Request{
		Subject: subject, Derivation: standardhistory.Derivation, Profile: evidencerequest.ProfileID,
		Coverage: evidencerequest.Coverage{MinFreshness: &evidencerequest.Freshness{Size: &size}},
	}, evidencerequest.MediaTypeJSON)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "request.json")
	require.NoError(t, os.WriteFile(path, octets, 0o600))
	return path
}

func respondCard(t *testing.T, request string) evidencerequest.Reply {
	t.Helper()
	out := filepath.Join(t.TempDir(), "reply.json")
	dealRun(t, "respond", "--request", request, "--requester", "the asking party", "--output", out)
	raw, err := os.ReadFile(out)
	require.NoError(t, err)
	reply, err := evidencerequest.ParseReply(evidencerequest.Message{Octets: raw, MediaType: evidencerequest.MediaTypeJSON})
	require.NoError(t, err)
	return reply
}

func refusedWith(t *testing.T, reply evidencerequest.Reply) string {
	t.Helper()
	require.NotNil(t, reply.Refusal, "the reply is a refusal")
	require.NoError(t, reply.Refusal.Verify())
	return reply.Refusal.Reason
}

var fullHistory = evidencerequest.Subject{Kind: evidencerequest.SubjectFullHistory}

// A node answers by default: the signed envelope's artifact is the card, its
// anchor is the cadence checkpoint the bundle's as_of names, and the shared
// bundle's block carries the same anchor and policy.
func TestANodeAnswersTheCard(t *testing.T) {
	dealFixture(t)
	now := clockAt(t, time.Date(2026, 10, 4, 18, 24, 0, 0, time.UTC))
	retailDeal(t)
	*now = now.Add(2 * time.Minute)
	require.Equal(t, "ticked", dealRun(t, "tick")["state"])
	dealRun(t, "standard-history", "declare", "--holder-kind", "node")

	reply := respondCard(t, cardRequest(t, fullHistory))
	require.NotNil(t, reply.Envelope, "an envelope, not a refusal")
	require.NoError(t, reply.Envelope.Verify())
	assert.Equal(t, standardhistory.Derivation, reply.Envelope.Derivation)

	shared := filepath.Join(t.TempDir(), "shared.json")
	dealRun(t, "standard-history", "card", "--out", shared, "--shared")
	block := shBlock(t, readBundle(t, shared))
	assert.Equal(t, block["as_of"].(map[string]any)["anchor"], reply.Envelope.Anchor.Digest)
	var card map[string]any
	require.NoError(t, json.Unmarshal(reply.Envelope.Artifact, &card))
	assert.Equal(t, block["policy"].(map[string]any)["digest"], card["policy"])
	assert.Equal(t, "node", card["holder_kind"])
	assert.Equal(t, reply.Envelope.Anchor.Digest, card["anchor"])
}

// Who is answered is the holder's policy, and what is not a choice is not
// said as one: a person's default and a node's opt-out decline, an
// undeclared holder does not serve the derivation, no tick is no anchor, and a
// subject form the derivation is not registered for has no such subject.
func TestTheCardRefusalsAreTheRightTokens(t *testing.T) {
	dealFixture(t)
	now := clockAt(t, time.Date(2026, 10, 4, 18, 24, 0, 0, time.UTC))
	retailDeal(t)
	request := cardRequest(t, fullHistory)

	assert.Equal(t, evidencebook.ReasonDerivationUnsupported, refusedWith(t, respondCard(t, request)), "undeclared")

	dealRun(t, "standard-history", "declare", "--holder-kind", "person")
	assert.Equal(t, evidencebook.ReasonPolicyDeclined, refusedWith(t, respondCard(t, request)), "a person answers only when set to")

	dealRun(t, "standard-history", "declare", "--holder-kind", "node", "--no-answer")
	assert.Equal(t, evidencebook.ReasonPolicyDeclined, refusedWith(t, respondCard(t, request)), "a node that opted out")

	dealRun(t, "standard-history", "declare", "--holder-kind", "node")
	assert.Equal(t, evidencebook.ReasonCoverageUnsatisfiable, refusedWith(t, respondCard(t, request)), "no finished tick yet")

	*now = now.Add(2 * time.Minute)
	require.Equal(t, "ticked", dealRun(t, "tick")["state"])
	require.NotNil(t, respondCard(t, request).Envelope)

	wrong := cardRequest(t, evidencerequest.Subject{Kind: evidencerequest.SubjectCheckpoints})
	assert.Equal(t, evidencebook.ReasonNoSuchSubject, refusedWith(t, respondCard(t, wrong)))

	// A person who turned answering on is answered.
	dealRun(t, "standard-history", "declare", "--holder-kind", "person", "--answer")
	require.NotNil(t, respondCard(t, request).Envelope)
}

// standardHistoryDir holds the golden bundles of a real run: a node's own
// copy and the copy it sends, with what `verify --bundle` printed for each.
// CAPSULECTL_UPDATE_STANDARD_HISTORY=1 rewrites them from a fresh run (keys and
// capsule ids are new each time); a normal run checks the committed files.
const standardHistoryDir = "../../testdata/standard-history"

// The golden bundles are VALID under `verify --bundle`, and each carries the
// block capsule-viewer's History module reads: exactly policy and as_of
// (as_of's three members), plus deals in the own copy only.
func TestTheStandardHistoryGoldens(t *testing.T) {
	if os.Getenv("CAPSULECTL_UPDATE_STANDARD_HISTORY") == "1" {
		dealFixture(t)
		now := clockAt(t, time.Date(2026, 10, 4, 18, 24, 0, 0, time.UTC))
		retailDeal(t)
		*now = now.Add(2 * time.Minute)
		require.Equal(t, "ticked", dealRun(t, "tick")["state"])
		dealRun(t, "standard-history", "declare", "--holder-kind", "node")
		require.NoError(t, os.MkdirAll(filepath.Join(standardHistoryDir, "verify"), 0o755))
		for name, extra := range map[string][]string{"node-own": nil, "node-shared": {"--shared"}} {
			path := filepath.Join(standardHistoryDir, name+".bundle.json")
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				require.NoError(t, err)
			}
			dealRun(t, append([]string{"standard-history", "card", "--out", path}, extra...)...)
			out, err := invoke(t, "", "verify", "--bundle", path)
			require.NoError(t, err, out)
			var v any
			require.NoError(t, json.Unmarshal([]byte(out), &v))
			pretty, err := json.MarshalIndent(v, "", "  ")
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(standardHistoryDir, "verify", name+".json"), append(pretty, '\n'), 0o644))
		}
	}
	for name, own := range map[string]bool{"node-own": true, "node-shared": false} {
		path := filepath.Join(standardHistoryDir, name+".bundle.json")
		result, err := verifyBundleOutput(t, path)
		require.NoError(t, err, name)
		assert.Equal(t, "VALID", result["verdict"], name)
		block := shBlock(t, readBundle(t, path))
		if own {
			assert.ElementsMatch(t, []string{"policy", "as_of", "deals"}, keysOf(block), name)
		} else {
			assert.ElementsMatch(t, []string{"policy", "as_of"}, keysOf(block), name)
		}
		assert.ElementsMatch(t, []string{"anchor", "anchor_time", "first_checkpoint_time"}, keysOf(block["as_of"].(map[string]any)), name+": no checkpoint or receipts member")
	}
}
