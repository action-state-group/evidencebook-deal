package deal

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The vectors are evidencebook's own (testdata/deal-v1/report-verify), a few
// copied: the plugin's rule is xdealv1.VerifyReport, so a report it accepts
// or refuses here is the one EvidenceBook does.
func TestVerifyV1ReportRule(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/deal-v1-report-verify/vectors.json")
	require.NoError(t, err)
	var vectors struct {
		Cases []struct {
			Name   string `json:"name"`
			Report string `json:"report"`
			First  string `json:"first"`
			Want   struct {
				KeyID string `json:"key_id"`
				Error string `json:"error"`
			} `json:"want"`
		} `json:"cases"`
	}
	require.NoError(t, json.Unmarshal(raw, &vectors))
	require.NotEmpty(t, vectors.Cases)
	for _, c := range vectors.Cases {
		t.Run(c.Name, func(t *testing.T) {
			report, err := hex.DecodeString(c.Report)
			require.NoError(t, err)
			first, err := hex.DecodeString(c.First)
			require.NoError(t, err)
			err = verifyV1Report(report, first)
			if c.Want.Error == "" {
				assert.NoError(t, err)
				return
			}
			assert.ErrorContains(t, err, c.Want.Error)
		})
	}
}
