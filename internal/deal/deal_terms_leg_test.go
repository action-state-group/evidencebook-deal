package deal

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func termsLegRun(t *testing.T, args ...string) (map[string]any, error) {
	t.Helper()
	out, err := invoke(t, "", append([]string{"deal", "terms-leg"}, args...)...)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &m), out)
	return m, nil
}

func TestATermsLegRoundTripsIntoTheSellersNotes(t *testing.T) {
	id, dueStep := sellerDealWithDueDelivery(t)
	path := filepath.Join(t.TempDir(), "terms.json")
	out, err := termsLegRun(t, "--ephemeral", "--amount", "1500000", "--content-digest", sha256Hex(deliveredContent), "--out", path)
	require.NoError(t, err)
	assert.Equal(t, path, out["terms_leg"])
	assert.Equal(t, true, out["ephemeral"])

	notePaymentReceived(t, id, path, x402Settle(true))
	dealRun(t, "note", "--deal", id, "--kind", "delivered", "--terms", path,
		"--content-digest", sha256Hex(deliveredContent), "--resolves", dueStep)

	got := dealSettlement(t, "--deal", id, "--leg", path)
	require.True(t, got.Settlement.Conforming, "%+v", got.Settlement.Failures)
	require.Len(t, got.Settlement.Settlements, 1)
	assert.Equal(t, "payee_stated", got.Settlement.Settlements[0].PaymentState)

	// The id it reports is the one the buyer's later legs name, and the
	// price it wrote equals what the buyer's own payer leg observed.
	buyer := newBuyerSide(t)
	both := dealSettlement(t, "--deal", id, "--leg", path, "--leg", buyer.payerObserved(out["capsule_id"].(string)),
		"--object", writeFile(t, "payload.json", x402PaymentPayload))
	require.True(t, both.Settlement.Conforming, "%+v", both.Settlement.Failures)
	assert.Equal(t, "equal", both.Settlement.Settlements[0].TermsAmount)
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"value":"1500000"`)
}

func TestATermsLegIsSignedWithTheProfileKeyUnlessEphemeral(t *testing.T) {
	dealFixture(t)
	dir := t.TempDir()
	digest := sha256Hex(deliveredContent)
	profile := func(name string) map[string]any {
		out, err := invoke(t, "", "--profile", "deal", "deal", "terms-leg", "--amount", "1", "--content-digest", digest, "--out", filepath.Join(dir, name))
		require.NoError(t, err, out)
		var m map[string]any
		require.NoError(t, json.Unmarshal([]byte(out), &m))
		return m
	}
	first, second := profile("a.json"), profile("b.json")
	assert.Equal(t, first["key_id"], second["key_id"], "the profile's key signs every leg")
	assert.Equal(t, false, first["ephemeral"])

	one, err := termsLegRun(t, "--ephemeral", "--amount", "1", "--content-digest", digest, "--out", filepath.Join(dir, "c.json"))
	require.NoError(t, err)
	two, err := termsLegRun(t, "--ephemeral", "--amount", "1", "--content-digest", digest, "--out", filepath.Join(dir, "d.json"))
	require.NoError(t, err)
	assert.NotEqual(t, one["key_id"], first["key_id"])
	assert.NotEqual(t, one["key_id"], two["key_id"], "an ephemeral key is made for each run")

	// Both verify as terms legs.
	_, err = termsLegID(filepath.Join(dir, "a.json"))
	assert.NoError(t, err)
}

func TestATermsLegNamesItsAsset(t *testing.T) {
	dir := t.TempDir()
	digest := sha256Hex(deliveredContent)
	path := filepath.Join(dir, "eth.json")
	_, err := termsLegRun(t, "--ephemeral", "--amount", "5", "--asset", "eip155:1/slip44:60", "--asset-scale", "18", "--content-digest", digest, "--out", path)
	require.NoError(t, err)
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"assetScale":18`)
	assert.Contains(t, string(raw), "eip155:1/slip44:60")
}

func TestATermsLegRefusesWhatItCannotStand(t *testing.T) {
	dir := t.TempDir()
	digest := sha256Hex(deliveredContent)
	out := filepath.Join(dir, "terms.json")
	run := func(args ...string) error {
		_, err := termsLegRun(t, append([]string{"--ephemeral"}, args...)...)
		return err
	}
	assert.ErrorContains(t, run("--content-digest", digest, "--out", out), "needs --amount")
	assert.ErrorContains(t, run("--amount", "1", "--out", out), "needs --amount")
	assert.ErrorContains(t, run("--amount", "1", "--content-digest", digest), "needs --amount")
	assert.ErrorContains(t, run("--amount", "1.5", "--content-digest", digest, "--out", out), "atomic units")
	assert.ErrorContains(t, run("--amount", "-1", "--content-digest", digest, "--out", out), "atomic units")
	assert.ErrorContains(t, run("--amount", "01", "--content-digest", digest, "--out", out), "atomic units")
	assert.ErrorContains(t, run("--amount", "1", "--content-digest", "ABC", "--out", out), "64 lowercase hex")
	assert.ErrorContains(t, run("--amount", "1", "--asset", "eip155:1/slip44:60", "--content-digest", digest, "--out", out), "--asset-scale")
	assert.ErrorContains(t, run("--amount", "1", "--asset-scale", "18", "--content-digest", digest, "--out", out), "goes with --asset")
	assert.ErrorContains(t, run("--amount", "1", "--asset", "not an asset", "--asset-scale", "6", "--content-digest", digest, "--out", out), "CAIP-19")
	_, err := os.Stat(out)
	assert.True(t, os.IsNotExist(err), "a leg the verifier refuses is not left behind")

	// Neither --ephemeral nor a profile: no key to sign with.
	_, err = invoke(t, "", "deal", "terms-leg", "--amount", "1", "--content-digest", digest, "--out", out)
	assert.ErrorContains(t, err, "--profile")
	_, err = invoke(t, "", "--profile", "deal", "deal", "terms-leg", "--ephemeral", "--amount", "1", "--content-digest", digest, "--out", out)
	assert.ErrorContains(t, err, "drop --profile")
}

func TestATermsLegDoesNotOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "terms.json")
	require.NoError(t, os.WriteFile(path, []byte("mine"), 0o600))
	_, err := termsLegRun(t, "--ephemeral", "--amount", "1", "--content-digest", sha256Hex(deliveredContent), "--out", path)
	assert.ErrorContains(t, err, "exists")
	got, _ := os.ReadFile(path)
	assert.Equal(t, "mine", string(got))
}
