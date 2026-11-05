package deal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/action-state-group/evidencebook/dealprofile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The dispute and the ruling (evidencebook-deal#151): two evidence steps, no new record family.

// adjudicatorCopies are two different files standing for the parties' adjudicator copies.
func adjudicatorCopies(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	a, b := filepath.Join(dir, "buyer.json"), filepath.Join(dir, "seller.json")
	require.NoError(t, os.WriteFile(a, []byte(`{"copy":"buyer"}`), 0o600))
	require.NoError(t, os.WriteFile(b, []byte(`{"copy":"seller"}`), 0o600))
	return a, b
}

func disputeNote(t *testing.T, id string) map[string]any {
	t.Helper()
	return dealRun(t, "note", "--deal", id, "--kind", "dispute", "--row", "price_as_offered", "--words", "the sticker was the wrong one")
}

func rulingNote(t *testing.T, id, disputeDigest, finding string, copies ...string) (map[string]any, error) {
	t.Helper()
	args := []string{"--profile", "deal", "deal", "note", "--deal", id, "--kind", "ruling", "--row", "price_as_offered", "--finding", finding,
		"--referee", "demo-referee", "--dispute-digest", disputeDigest}
	for _, c := range copies {
		args = append(args, "--copy", c)
	}
	out, err := invoke(t, "", args...)
	if err != nil {
		return nil, err
	}
	return map[string]any{"raw": out}, nil
}

const someDigest = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestADisputeAndOneRulingOnTheBuyersDeal(t *testing.T) {
	dealFixture(t)
	id := retailDeal(t)
	a, b := adjudicatorCopies(t)

	d := disputeNote(t, id)
	disputeDigest, _ := d["dispute_digest"].(string)
	require.Len(t, disputeDigest, 64)
	_, err := invoke(t, "", "--profile", "deal", "deal", "note", "--deal", id, "--kind", "dispute", "--row", "price_as_offered", "--words", "again")
	require.ErrorContains(t, err, "one dispute per deal")

	ruling := dealRun(t, "note", "--deal", id, "--kind", "ruling", "--row", "price_as_offered", "--finding", "for_buyer",
		"--referee", "demo-referee", "--dispute-digest", disputeDigest, "--copy", a, "--copy", b)
	digest, _ := ruling["ruling_digest"].(string)
	require.Len(t, digest, 64)
	assert.Contains(t, ruling["say"], `The referee ruled on the dispute over "price as offered": found for the buyer. Ruling `+digest[:rulingShown])

	_, err = rulingNote(t, id, disputeDigest, "for_seller", a, b)
	require.ErrorContains(t, err, "already holds a ruling")
}

func TestTheRulingDigestIsTheSameWhicheverStoreItIsWrittenTo(t *testing.T) {
	a, b := adjudicatorCopies(t)
	var digests []string
	for range 2 {
		dealFixture(t)
		id := retailDeal(t)
		// the copies in the other order: the body sorts them.
		first, second := a, b
		if len(digests) == 1 {
			first, second = b, a
		}
		r := dealRun(t, "note", "--deal", id, "--kind", "ruling", "--row", "price_as_offered", "--finding", "for_buyer",
			"--referee", "demo-referee", "--dispute-digest", someDigest, "--copy", first, "--copy", second)
		digests = append(digests, r["ruling_digest"].(string))
	}
	assert.Equal(t, digests[0], digests[1])
}

func TestTheDisputeAndRulingFlagsAreChecked(t *testing.T) {
	dealFixture(t)
	id := retailDeal(t)
	a, b := adjudicatorCopies(t)
	run := func(args ...string) error {
		_, err := invoke(t, "", append([]string{"--profile", "deal", "deal", "note", "--deal", id}, args...)...)
		return err
	}
	assert.ErrorContains(t, run("--kind", "dispute", "--words", "x"), "--row")
	assert.ErrorContains(t, run("--kind", "dispute", "--row", "price_as_offered"), "--words")
	assert.ErrorContains(t, run("--kind", "ruling", "--row", "r", "--finding", "maybe", "--referee", "x", "--dispute-digest", someDigest, "--copy", a, "--copy", b), "for_buyer or for_seller")
	assert.ErrorContains(t, run("--kind", "ruling", "--row", "r", "--finding", "for_buyer", "--dispute-digest", someDigest, "--copy", a, "--copy", b), "--referee")
	assert.ErrorContains(t, run("--kind", "ruling", "--row", "r", "--finding", "for_buyer", "--referee", "x", "--dispute-digest", "abc", "--copy", a, "--copy", b), "--dispute-digest")
	assert.ErrorContains(t, run("--kind", "ruling", "--row", "r", "--finding", "for_buyer", "--referee", "x", "--dispute-digest", someDigest, "--copy", a), "two --copy")
	assert.ErrorContains(t, run("--kind", "ruling", "--row", "r", "--finding", "for_buyer", "--referee", "x", "--dispute-digest", someDigest, "--copy", a, "--copy", a), "same")
	// Nothing but the two kinds seals these sources.
	assert.ErrorContains(t, run("--kind", "evidence", "--input", writeJSON(t, `{"about":"price as offered","source":"ruling_for_buyer"}`)), "sealed only by")
}

func TestOnlyTheBuyerDisputes(t *testing.T) {
	dealFixture(t)
	id := openTyped(t, sellerTyped)
	_, err := invoke(t, "", "--profile", "deal", "deal", "note", "--deal", id, "--kind", "dispute", "--row", "price_as_offered", "--words", "x")
	require.ErrorContains(t, err, "the buyer disputes")
}

// Both parties' own receipts read the one line, and the counterparty's shared copy carries the same
// words and digest (the ruling's source token is allowed through the share gate).
func TestTheReceiptShowsTheRulingLine(t *testing.T) {
	dealFixture(t)
	id := retailDeal(t)
	a, b := adjudicatorCopies(t)
	d := disputeNote(t, id)
	r := dealRun(t, "note", "--deal", id, "--kind", "ruling", "--row", "price_as_offered", "--finding", "for_buyer",
		"--referee", "demo-referee", "--dispute-digest", d["dispute_digest"].(string), "--copy", a, "--copy", b)
	line := "Ruling " + r["ruling_digest"].(string)[:rulingShown] + ` on \"price as offered\": found for the buyer`
	assert.Contains(t, receiptText(t, id, false), line)
	assert.Contains(t, receiptText(t, id, true), line)
}

func TestTheCardCountsTheRulingForTheBuyer(t *testing.T) {
	for finding, want := range map[string]string{"for_buyer": "for", "for_seller": "against"} {
		dealFixture(t)
		now := clockAt(t, time.Date(2026, 10, 4, 18, 24, 0, 0, time.UTC))
		id := retailDeal(t)
		a, b := adjudicatorCopies(t)
		d := disputeNote(t, id)
		dealRun(t, "note", "--deal", id, "--kind", "ruling", "--row", "price_as_offered", "--finding", finding,
			"--referee", "demo-referee", "--dispute-digest", d["dispute_digest"].(string), "--copy", a, "--copy", b)
		dealRun(t, "standard-history", "declare", "--holder-kind", "node")
		*now = now.Add(2 * time.Minute)
		require.Equal(t, "ticked", dealRun(t, "tick")["state"])
		own := filepath.Join(t.TempDir(), "own.json")
		dealRun(t, "standard-history", "card", "--out", own)
		entry := shBlock(t, readBundle(t, own))["deals"].([]any)[0].(map[string]any)
		assert.Equal(t, []any{map[string]any{"side": "raised", "outcome": want, "forum": "external"}}, entry["disputes"], finding)
	}
}

// The seller's store holds only the ruling; a buyer's store with a dispute and no ruling yet counts it open.
func TestDisputesOfReadsTheRecordsUnderTheAnchor(t *testing.T) {
	rec := func(source string) dealprofile.HistoryRecord {
		return dealprofile.HistoryRecord{Payload: []byte(`{"x-deal-v0":{"record_type":"evidence"},"body":{"source":"` + source + `"}}`)}
	}
	other := dealprofile.HistoryRecord{Payload: []byte(`{"x-deal-v0":{"record_type":"message"},"body":{"source":"ruling_for_buyer"}}`)}
	d := func(side, outcome string) string { return side + "/" + outcome }
	got := func(recs []dealprofile.HistoryRecord, role string) string {
		ds := disputesOf(recs, role)
		var out []string
		for _, x := range ds {
			out = append(out, d(x.Side, x.Outcome))
		}
		return strings.Join(out, ",")
	}
	assert.Equal(t, "received/against", got([]dealprofile.HistoryRecord{rec("ruling_for_buyer")}, "provided"))
	assert.Equal(t, "received/for", got([]dealprofile.HistoryRecord{rec("ruling_for_seller")}, "provided"))
	assert.Equal(t, "raised/for", got([]dealprofile.HistoryRecord{rec("dispute"), rec("ruling_for_buyer")}, "bought"))
	assert.Equal(t, "raised/open", got([]dealprofile.HistoryRecord{rec("dispute")}, "bought"))
	assert.Equal(t, "", got([]dealprofile.HistoryRecord{rec("dispute")}, "provided"), "a seller's store holds no dispute of its own")
	assert.Equal(t, "", got([]dealprofile.HistoryRecord{other}, "bought"), "only an evidence record counts")
	assert.Equal(t, "", got(nil, "bought"))
}
