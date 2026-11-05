package deal

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/action-state-group/capsule-cli/cli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The user's own copy opens who the deal was with and what the buyer wrote
// (evidencebook-deal#56). The page recomputes each against the sealed
// fingerprint or content_commitment; a shared copy carries none of it, nor
// the deal key.

const buyerWords = "Deal, that works for me"

// acceptedSale seals a seller's deal whose buyer accepted the offer, and
// returns its id.
func acceptedSale(t *testing.T) string {
	t.Helper()
	dealFixture(t)
	id := openTyped(t, sellerTyped)
	check := makeOffer(t, id, "190000")
	_, err := acceptOffer(t, id, check)
	require.NoError(t, err)
	return id
}

func ownReport(t *testing.T, id string) map[string]interface{} {
	t.Helper()
	path := filepath.Join(t.TempDir(), "own.json")
	dealRun(t, "report", "--deal", id, "--bundle", path)
	return bundleFile(t, path)
}

func sealedReport(t *testing.T, b map[string]interface{}) map[string]interface{} {
	t.Helper()
	r, _, _ := cli.DealSealedReport(b)
	require.NotNil(t, r)
	return r
}

func TestTheOwnCopyOpensTheCounterpartyAndTheBuyersWords(t *testing.T) {
	id := acceptedSale(t)
	b := ownReport(t, id)
	r := sealedReport(t, b)

	cp, _ := r["counterparty_openings"].(map[string]interface{})
	require.NotNil(t, cp)
	assert.NotEmpty(t, cp["key"])
	var names, domains int
	for _, e := range cp["steps"].([]interface{}) {
		entry := e.(map[string]interface{})
		if entry["name"] != nil {
			names++
			assert.Equal(t, "Example Buyer", entry["name"])
		}
		if entry["domain"] != nil {
			domains++
			assert.Equal(t, "buyer.example", entry["domain"])
		}
	}
	assert.Positive(t, names)
	assert.Positive(t, domains)

	acc := r["acceptance_openings"].([]interface{})
	require.Len(t, acc, 1)
	assert.Equal(t, buyerWords, acc[0].(map[string]interface{})["text"])

	out, err := dealPageOpenings(b)
	require.NoError(t, err)
	for step, kinds := range out["counterparty"].(map[string]interface{}) {
		for kind, ok := range kinds.(map[string]interface{}) {
			assert.Equal(t, true, ok, "%s %s", step, kind)
		}
	}
	assert.NotEmpty(t, out["counterparty"])
	for step, ok := range out["acceptances"].(map[string]interface{}) {
		assert.Equal(t, true, ok, step)
	}
	assert.Len(t, out["acceptances"], 1)
}

// An opening that does not recompute is reported not verified: other words, a
// name or domain that was not sealed, another key or nonce, or a step that
// is not the acceptance. The page then shows none of the value.
func TestAnOpeningThatDoesNotMatchIsNotVerified(t *testing.T) {
	id := acceptedSale(t)
	path := filepath.Join(t.TempDir(), "own.json")
	dealRun(t, "report", "--deal", id, "--bundle", path)

	for name, tamper := range map[string]func(r map[string]interface{}){
		"other name": func(r map[string]interface{}) {
			r["counterparty_openings"].(map[string]interface{})["steps"].([]interface{})[0].(map[string]interface{})["name"] = "Someone Else"
		},
		"other domain": func(r map[string]interface{}) {
			e := r["counterparty_openings"].(map[string]interface{})["steps"].([]interface{})[0].(map[string]interface{})
			if e["domain"] != nil {
				e["domain"] = "else.example"
			} else {
				e["name"] = "Someone Else"
			}
		},
		"other key": func(r map[string]interface{}) {
			r["counterparty_openings"].(map[string]interface{})["key"] = strings.Repeat("0", 64)
		},
		"no key": func(r map[string]interface{}) { delete(r["counterparty_openings"].(map[string]interface{}), "key") },
		"other words": func(r map[string]interface{}) {
			r["acceptance_openings"].([]interface{})[0].(map[string]interface{})["text"] = "Not what they said"
		},
		"other nonce": func(r map[string]interface{}) {
			r["acceptance_openings"].([]interface{})[0].(map[string]interface{})["nonce"] = strings.Repeat("0", 64)
		},
		"no such step": func(r map[string]interface{}) {
			r["acceptance_openings"].([]interface{})[0].(map[string]interface{})["step"] = "nope"
		},
	} {
		t.Run(name, func(t *testing.T) {
			b := bundleFile(t, path)
			r := sealedReport(t, b)
			tamper(r)
			out, err := dealPageOpenings(b)
			require.NoError(t, err, "the page is written; the value is not shown")
			failed := false
			for _, kinds := range out["counterparty"].(map[string]interface{}) {
				for _, ok := range kinds.(map[string]interface{}) {
					failed = failed || ok == false
				}
			}
			for _, ok := range out["acceptances"].(map[string]interface{}) {
				failed = failed || ok == false
			}
			assert.True(t, failed, "something does not verify")
		})
	}
}

// A shared copy carries neither the openings or the deal key
// the deal key that would let a holder test a guessed name against
// counterparty.ids.name, and a shared copy that did carry them refuses the page.
func TestSharedCopiesCarryNoCounterpartyOpenings(t *testing.T) {
	id := acceptedSale(t)
	dir := t.TempDir()
	own := filepath.Join(dir, "own.json")
	dealRun(t, "report", "--deal", id, "--bundle", own)
	ownReport := sealedReport(t, bundleFile(t, own))
	key := ownReport["counterparty_openings"].(map[string]interface{})["key"].(string)
	nonce := ownReport["acceptance_openings"].([]interface{})[0].(map[string]interface{})["nonce"].(string)
	require.NotEmpty(t, key)
	require.NotEmpty(t, nonce)

	for _, audience := range []string{cli.DealAudienceCounterparty, cli.DealAudienceAdjudicator} {
		html := filepath.Join(dir, audience+".html")
		dealRun(t, "report", "--deal", id, "--share", audience, "--to", "them", "--html", html)
		raw := string(mustRead(t, html))
		assert.NotContains(t, raw, key, audience)
		assert.NotContains(t, raw, nonce, audience)
		r := sealedReport(t, embeddedBundle(t, raw))
		assert.NotContains(t, r, "counterparty_openings")
		assert.NotContains(t, r, "acceptance_openings")

		for field, value := range map[string]interface{}{
			"counterparty_openings": map[string]interface{}{"key": key, "steps": []interface{}{}},
			"acceptance_openings":   []interface{}{},
		} {
			b := embeddedBundle(t, raw)
			sealedReport(t, b)[field] = value
			_, err := dealPageOpenings(b)
			require.Error(t, err, "%s on a %s copy", field, audience)
			assert.NotContains(t, err.Error(), key)
		}
	}
}

// A deal sealed with no name, domain or acceptance to open reports, and
// renders, as it did.
func TestADealWithNothingToOpenCarriesNoOpenings(t *testing.T) {
	dealFixture(t)
	id := dealRun(t, "open", "--input", writeJSON(t, `{"type":"purchase","channel":"web",
		"intent":{"verbatim":"buy me an otter sticker under 8 bucks","max_total_minor":800,"allowed":["pay"]},
		"who":{"payee":"Sticker Marketplace"},
		"terms":{"item":"otter sticker","price_minor":600,"currency":"USD"},
		"recourse":{"rail":"card","refundable":true}}`))["deal_id"].(string)
	b := ownReport(t, id)
	r := sealedReport(t, b)
	assert.NotContains(t, r, "counterparty_openings")
	assert.NotContains(t, r, "acceptance_openings")
	out, err := dealPageOpenings(b)
	require.NoError(t, err)
	assert.NotContains(t, out, "counterparty")
	assert.NotContains(t, out, "acceptances")
}
