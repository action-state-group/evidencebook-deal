package deal

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The rules checker validates a checker's output against the profile's
// external-check-result-v0 schema. EvidenceBook ships it with the profile
// (dealprofile/x-deal-v0) but does not embed it, so the checker embeds a
// copy, and that copy must stay the profile's bytes.
func TestExternalCheckSchemaIsTheProfiles(t *testing.T) {
	shipped, err := os.ReadFile(filepath.Join(dealProfileDir, "external-check-result-v0.schema.json"))
	require.NoError(t, err)
	assert.Equal(t, string(shipped), string(externalCheckSchemaJSON))
}
