package deal

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The pack that judged a check is named on the receipt (evidencebook-deal#138): on the user's own
// copy always, and on a counterparty's copy only when the profile pins that pack as the party's
// deal terms (kind "deal"). A conduct pack, or a pin that states no kind, reaches no one.

const (
	packID     = "sticker-sale/1"
	packDigest = "a81f3c9d2e10b4c7d5e6f708192a3b4c5d6e7f8091a2b3c4d5e6f708192a3b4c"
	packShown  = "a81f3c9d2e10"
)

// packChecked pins a stub checker that reports the sticker pack as evaluated, with the given
// kind in the pin ("" states none), and runs one check on a new seller or buyer deal.
func packChecked(t *testing.T, kind, role string) string {
	t.Helper()
	dealFixture(t)
	out := `printf '%s' '{"schema":"external-check-result/v0","ruleset_id":"` + packID + `","definition_digest":"` + packDigest +
		`","verdict":"allow","tier":"recomputed","findings":[{"id":"price_as_offered","check":"price equals the offer","verdict":"pass"}]}'`
	pin := map[string]any{"command": []string{stubChecker(t, "pack", out)}, "definition_digest": packDigest}
	if kind != "" {
		pin["kind"] = kind
	}
	pinChecker(t, pin)
	if role == dealRoleSeller {
		id := openTyped(t, sellerTyped)
		dealRun(t, "check", "--deal", id, "--input", writeJSON(t,
			`{"action":"offer","amount_minor":190000,"terms":{"item":"example bicycle","quantity":1,"price_minor":190000}}`))
		return id
	}
	id := openTypedSticker(t)
	checkPay(t, id, 600)
	return id
}

// receiptText is every word of a copy's report, own or shared: the page's embedded bundle
// as the viewer draws it.
func receiptText(t *testing.T, id string, shared bool) string {
	t.Helper()
	page := filepath.Join(t.TempDir(), "copy.html")
	args := []string{"report", "--deal", id, "--html", page}
	if shared {
		args = append(args, "--share", "counterparty", "--to", "the other side")
	}
	dealRun(t, args...)
	raw, err := os.ReadFile(page)
	require.NoError(t, err)
	text, err := json.Marshal(dealReportOf(embeddedBundle(t, string(raw))))
	require.NoError(t, err)
	return string(text)
}

func TestTheOwnReceiptNamesThePackThatJudgedTheCheck(t *testing.T) {
	for _, kind := range []string{"", "conduct", "deal"} {
		id := packChecked(t, kind, dealRoleSeller)
		assert.Contains(t, receiptText(t, id, false), "under "+packID+", pack "+packShown, "kind %q", kind)
	}
}

// The seller's copy goes to the buyer, so the buyer's receipt reads the digest from the seller's
// half and names it as the seller's terms, never as a check the buyer ran.
func TestAPartysDealTermsReachTheCounterpartyAsTheirTerms(t *testing.T) {
	copyText := receiptText(t, packChecked(t, "deal", dealRoleSeller), true)
	assert.Contains(t, copyText, "under the seller's terms, pack "+packShown)
	assert.NotContains(t, copyText, "under "+packID, "the ruleset id is the owner's own wording")
	assert.NotContains(t, copyText, packDigest, "only the shown prefix")

	buyer := receiptText(t, packChecked(t, "deal", dealRoleBuyer), true)
	assert.Contains(t, buyer, "under the buyer's terms, pack "+packShown)
}

// Steven's PRD rule: a conduct pack never appears in a copy sent to a counterparty. Nor does a
// pack whose pin states no kind, or any other kind.
func TestAPackThatIsNotDealTermsNeverReachesTheCounterparty(t *testing.T) {
	for _, kind := range []string{"", "conduct", "admission", "live", "after_the_fact"} {
		id := packChecked(t, kind, dealRoleSeller)
		copyText := receiptText(t, id, true)
		for _, leak := range []string{packID, packShown, packDigest, "pack ", "terms, pack"} {
			assert.NotContains(t, copyText, leak, "kind %q", kind)
		}
		assert.True(t, strings.Contains(receiptText(t, id, false), packShown), "the owner's own copy still names it (kind %q)", kind)
	}
}
