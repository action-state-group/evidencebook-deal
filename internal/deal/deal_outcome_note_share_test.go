package deal

import (
	"encoding/json"
	"testing"

	"github.com/action-state-group/capsule-cli/cli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// u597 (evidencebook-deal#218): the temporary outcome_note is never in a
// shared copy. A counterparty copy lists the record withheld, as it lists a
// withheld rules_verdict, so the log's records are all accounted for; the
// listed entry must not say which late record it is.
func TestAWithheldOutcomeNoteIsListedLikeAWithheldRulesVerdict(t *testing.T) {
	id := closedUnderPack(t, packID, "")
	sealVerdict(t, id)
	dealRun(t, "outcome-note", "seal", "--deal", id, "--reason", "dispute_lost")

	b, raw := sharedCopy(t, id, cli.DealAudienceCounterparty, "the other side")
	assert.NotContains(t, raw, "dispute_lost")
	assert.NotContains(t, raw, "did_not_hold")
	assert.NotContains(t, raw, "outcome_note")

	records, _ := b["records"].([]any)
	require.GreaterOrEqual(t, len(records), 2)
	verdict, note := records[len(records)-2].(map[string]any), records[len(records)-1].(map[string]any)
	vs, ns := valueShape(verdict), valueShape(note)
	t.Logf("verdict entry: %s", jsonText(t, verdict))
	t.Logf("note entry:    %s", jsonText(t, note))
	assert.Equal(t, vs, ns, "the two late records list the same members")

	steps := sharedSteps(t, b)
	require.GreaterOrEqual(t, len(steps), 2)
	v, n := steps[len(steps)-2], steps[len(steps)-1]
	assert.Equal(t, dealSharePrivateKind, v["kind"])
	assert.Equal(t, dealSharePrivateKind, n["kind"])
	assert.Equal(t, v["line"], n["line"])
	assert.Equal(t, true, n["withheld"])
}

func jsonText(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	require.NoError(t, err)
	return string(raw)
}

// valueShape is a JSON value with every string and number replaced by its
// type: the members and nesting a record lists, not its digests and ids.
func valueShape(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, e := range x {
			out[k] = valueShape(e)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = valueShape(e)
		}
		return out
	case string:
		return "string"
	case float64:
		return "number"
	}
	return v
}
