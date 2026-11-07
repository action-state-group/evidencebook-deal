package deal

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func heldList(t *testing.T) []any {
	t.Helper()
	return runJSON(t, "deal", "held")["held"].([]any)
}

// The holder lists a held request, declines it with a signed policy_declined
// refusal, and it leaves the list; declining again rewrites the refusal and
// records nothing more (#178).
func TestDealHeldListAndDecline(t *testing.T) {
	dealFixture(t)
	id := dealRun(t, "open", "--input", writeJSON(t, crossPartyBuyerOpen))["deal_id"].(string)
	assert.Empty(t, heldList(t))
	request := writeJSONFile(t, "req.json", map[string]any{"subject": map[string]any{"kind": "correlation", "value": id},
		"coverage": map[string]any{"min_freshness": map[string]any{"size": 1}}, "derivation": "outcome/1"})
	scratch := t.TempDir()
	reply := filepath.Join(scratch, "response.json")
	held := runJSON(t, "deal", "respond", "--request", request, "--requester", "x", "--output", reply)
	digest := held["request_digest"].(string)

	list := heldList(t)
	require.Len(t, list, 1)
	item := list[0].(map[string]any)
	assert.Equal(t, digest, item["request_digest"])
	assert.Equal(t, "x", item["requester"])
	assert.Equal(t, "outcome/1", item["derivation"])
	assert.Equal(t, id, item["receipt_id"])
	assert.Equal(t, "held, can be declined", item["state"])

	out := filepath.Join(scratch, "refusal.json")
	got := runJSON(t, "deal", "decline", "--request-digest", digest, "--output", out)
	assert.Equal(t, "refusal", got["outcome"])
	assert.Equal(t, "policy_declined", got["reason"])
	assert.NotEmpty(t, got["record_id"])
	raw, err := os.ReadFile(out)
	require.NoError(t, err)
	var resp struct {
		Refusal struct {
			RequestDigest string `json:"request_digest"`
			Reason        string `json:"reason"`
			Sig           string `json:"sig"`
		} `json:"refusal"`
	}
	require.NoError(t, json.Unmarshal(raw, &resp))
	assert.Equal(t, digest, resp.Refusal.RequestDigest)
	assert.Equal(t, "policy_declined", resp.Refusal.Reason)
	assert.NotEmpty(t, resp.Refusal.Sig)
	assert.Empty(t, heldList(t), "a declined request is no longer pending")

	again := runJSON(t, "deal", "decline", "--request-digest", digest, "--output", filepath.Join(scratch, "refusal2.json"))
	assert.Empty(t, again["record_id"], "declining twice records nothing more")
	require.Error(t, dealRunErr(t, "respond", "--request", request, "--requester", "x", "--output", reply), "a declined request is not held again")
}

func TestDealDeclineInteropShapeAndErrors(t *testing.T) {
	dealFixture(t)
	scratch := t.TempDir()
	interop := writeFile(t, "interop.json", []byte(`{"subject":{"full_history":null},"coverage":{"min_freshness":1},"derivation":"outcome/1","profile":"evidence-interop/0"}`))
	digest := runJSON(t, "deal", "respond", "--request", interop, "--requester", "x", "--output", filepath.Join(scratch, "r.json"))["request_digest"].(string)
	out := filepath.Join(scratch, "refusal.json")
	assert.Equal(t, "policy_declined", runJSON(t, "deal", "decline", "--request-digest", digest, "--output", out)["reason"])
	raw, err := os.ReadFile(out)
	require.NoError(t, err)
	assert.Contains(t, string(raw), "policy_declined")
	assert.Contains(t, string(raw), digest)

	require.Error(t, dealRunErr(t, "decline", "--request-digest", "0000", "--output", out), "not a held request")
	require.Error(t, dealRunErr(t, "decline", "--output", out), "digest required")
	require.Error(t, dealRunErr(t, "decline", "--request-digest", digest), "output required")
}
