package deal

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/action-state-group/capsule-cli/cli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// exportSources reads an export file's records and returns each evidence
// record's body.source token, the clear field that carries a dispute and its
// ruling's side.
func exportSources(t *testing.T, path string) []string {
	t.Helper()
	var records []struct {
		X struct {
			RecordType string `json:"record_type"`
		} `json:"x-deal-v0"`
		Body struct {
			Source string `json:"source"`
		} `json:"body"`
	}
	require.NoError(t, json.Unmarshal(mustRead(t, path), &records))
	var out []string
	for _, r := range records {
		if r.X.RecordType == "evidence" {
			out = append(out, r.Body.Source)
		}
	}
	return out
}

// `deal export --read-only` gives a read-only consumer the sealed dispute and
// the side of the ruling as the same records a write session exports, and the
// read changes no byte of the store.
func TestDealExportReadOnlyGivesTheDisputeAndSide(t *testing.T) {
	dealFixture(t)
	id := retailDeal(t)
	a, b := adjudicatorCopies(t)
	disputeDigest, _ := disputeNote(t, id)["dispute_digest"].(string)
	_, err := rulingNote(t, id, disputeDigest, "for_seller", a, b)
	require.NoError(t, err)

	written := filepath.Join(t.TempDir(), "written.json")
	dealRun(t, "export", "--deal", id, "--output", written)

	p, err := cli.LoadProfile("deal")
	require.NoError(t, err)
	before := storeFiles(t, p)
	read := filepath.Join(t.TempDir(), "read.json")
	out := dealRun(t, "export", "--deal", id, "--output", read, "--read-only")
	assert.Equal(t, id, out["deal_id"])
	assert.Equal(t, before, storeFiles(t, p), "a read writes nothing to the store")
	assert.Equal(t, mustRead(t, written), mustRead(t, read))
	assert.Subset(t, exportSources(t, read), []string{"dispute", "ruling_for_seller"})
}

// The read takes no writer lock: the session it runs over is no writer
// (HoldsPluginStore), so the core would refuse it any write to the store.
func TestDealExportReadOnlyTakesNoWriterLock(t *testing.T) {
	dealFixture(t)
	id := retailDeal(t)
	disputeNote(t, id)
	p, err := cli.LoadProfile("deal")
	require.NoError(t, err)
	s, err := openDealSessionRead(t.Context(), p, id)
	require.NoError(t, err)
	assert.False(t, s.HoldsPluginStore(), "a read holds no writer lock")
	require.NoError(t, s.close())

	out := filepath.Join(t.TempDir(), "read.json")
	dealRun(t, "export", "--deal", id, "--output", out, "--read-only")
	assert.Contains(t, exportSources(t, out), "dispute")
}

// The read needs no signing key: with the profile's keys removed it still
// exports, where a write session cannot seal.
func TestDealExportReadOnlyNeedsNoSigningKey(t *testing.T) {
	dealFixture(t)
	id := retailDeal(t)
	disputeNote(t, id)
	p, err := cli.LoadProfile("deal")
	require.NoError(t, err)
	p.Signing = cli.Secret{}
	require.NoError(t, cli.SaveProfile(p, true))
	out := filepath.Join(t.TempDir(), "read.json")
	dealRun(t, "export", "--deal", id, "--output", out, "--read-only")
	assert.Contains(t, exportSources(t, out), "dispute")
}

// The read never migrates: a store no build with the shelf has opened is
// refused and left exactly as it was.
func TestDealExportReadOnlyNeverMigrates(t *testing.T) {
	p, deals, _ := rc16Store(t)
	before := storeFiles(t, p)
	out := filepath.Join(t.TempDir(), "read.json")
	_, err := invoke(t, "", "--profile", "deal", "deal", "export", "--deal", deals.Closed, "--output", out, "--read-only")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not moved onto its shelf")
	assert.Equal(t, before, storeFiles(t, p))
	assert.NoFileExists(t, out)
}
