package deal

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A terms pack the user wrote is of kind "deal": its checker states each row in the pack's own
// words, and the seller's card says them by name instead of "your rules" (evidencebook-deal#157).
const (
	pendingRow = `{"id":"price_as_offered","check":"price_as_offered","verdict":"not_evaluable","reason":"your rule \"The price was the price offered\" is checked when the buyer commits"}`
	failedRow  = `{"id":"price_as_offered","check":"price_as_offered","verdict":"fail","disposition":"ask","reason":"your rule \"The price was the price offered\" did not hold: the commit was $0.60, the offer $0.50","value":60,"limit":50}`
)

func packCard(t *testing.T, kind, verdict, finding string) string {
	t.Helper()
	dealFixture(t)
	saleID, _ := newSale(t)
	id := buyerThread(t, saleID, "buyer.example")
	pin := map[string]any{"command": []string{stubChecker(t, "pack-words", checkerPrints(verdict, finding))}}
	if kind != "" {
		pin["kind"] = kind
	}
	pinChecker(t, pin)
	card, _ := dealCheckOf(t, id, phoneShare)["card"].(string)
	return card
}

func TestAPackRowIsNamedOnTheCard(t *testing.T) {
	pending := packCard(t, "deal", "not_evaluable", pendingRow)
	assert.Contains(t, pending, `your rule "The price was the price offered" is checked when the buyer commits`)
	assert.NotContains(t, pending, "I couldn't check this against your rules", "a pending row is not a false alarm")

	failed := packCard(t, "deal", "deny", failedRow)
	require.Contains(t, failed, "NOT ALLOWED BY YOUR RULES")
	assert.Contains(t, failed, `your rule "The price was the price offered" did not hold: the commit was $0.60, the offer $0.50`)
	assert.NotContains(t, failed, "your rules don't allow this")
}

func TestOnlyADealPacksWordsReachTheCard(t *testing.T) {
	for _, kind := range []string{"", "live"} {
		card := packCard(t, kind, "not_evaluable", pendingRow)
		assert.Contains(t, card, "I couldn't check this against your rules", kind)
		assert.NotContains(t, card, "The price was the price offered", kind)
	}
	assert.Contains(t, packCard(t, "", "deny", failedRow), "your rules don't allow this")
}
