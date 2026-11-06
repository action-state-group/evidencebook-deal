package deal

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// rulesBlocks collects every "rules" member under v, in document order.
func rulesBlocks(v any) []any {
	var out []any
	switch x := v.(type) {
	case map[string]any:
		if r, ok := x["rules"]; ok {
			out = append(out, r)
		}
		for _, k := range sortedKeys(x) {
			out = append(out, rulesBlocks(x[k])...)
		}
	case []any:
		for _, e := range x {
			out = append(out, rulesBlocks(e)...)
		}
	}
	return out
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	for i := range keys {
		for j := i + 1; j < len(keys); j++ {
			if keys[j] < keys[i] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	return keys
}

// Cards handed to the rules checker at a check change nothing that is sealed
// (evidencebook-deal#166): the sealed rules block is the same with and without
// them, neither the cards nor the checker's notice reach any exported record,
// and the notice is shown in the check's output only.
func TestReceivedCardsReachTheCheckerAndSealNothing(t *testing.T) {
	const marker = "CARD-MARKER-9f3a"
	cards := writeJSON(t, `[{"schema":"history_card/1","counterparty":"`+marker+`"},{"schema":"selection/1"}]`)
	id := stickerDeal(t, "card", false)
	pinStub(t, "1000")
	input := writeJSON(t, payCheck("card", 454, 480))

	without := dealRun(t, "check", "--deal", id, "--input", input)
	with := dealRun(t, "check", "--deal", id, "--input", input, "--received", cards)

	assert.NotContains(t, without, "rules_notice")
	assert.Equal(t, "read 2 received card(s)", with["rules_notice"], "the checker was given both cards and its line is shown")
	assert.Equal(t, without["verdict"], with["verdict"])
	assert.Equal(t, without["rules"], with["rules"], "the rules block the check reports is the same with and without cards")

	records, _ := exportRecords(t, id)
	var all []any
	for _, r := range records {
		all = append(all, r)
	}
	blocks := rulesBlocks(all)
	require.Len(t, blocks, 2, "one sealed rules block per check")
	assert.Equal(t, blocks[0], blocks[1], "the sealed rules blocks are identical")
	raw, err := json.Marshal(records)
	require.NoError(t, err)
	for _, leaked := range []string{marker, "received card", "received_cards", "notice"} {
		assert.False(t, strings.Contains(string(raw), leaked), "the sealed export holds %q", leaked)
	}
}

// --received takes a non-empty array of card objects and nothing else.
func TestReceivedCardsAreRefusedUnlessAnArrayOfCards(t *testing.T) {
	id := stickerDeal(t, "card", false)
	pinStub(t, "1000")
	for name, body := range map[string]string{
		"an empty array": `[]`,
		"an object":      `{"schema":"history_card/1"}`,
		"a string card":  `["card"]`,
		"a null card":    `[null]`,
		"65 cards":       `[` + strings.TrimSuffix(strings.Repeat(`{},`, 65), ",") + `]`,
		"over 1 MiB":     `[{"x":"` + strings.Repeat("a", 1<<20) + `"}]`,
	} {
		err := dealRunErr(t, "check", "--deal", id, "--input", writeJSON(t, payCheck("card", 454, 480)), "--received", writeJSON(t, body))
		assert.Error(t, err, name)
	}
}
