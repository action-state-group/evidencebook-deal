package deal

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"testing"

	"github.com/action-state-group/capsule-cli/cli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPinnedWitnessDirectoryTrustsOnlyTheProfilesWitness(t *testing.T) {
	public, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	var p cli.Profile
	p.Checkpoint.Endpoint = "https://witness.example/"
	p.Checkpoint.PublicKey = hex.EncodeToString(public)

	directory, err := pinnedWitnessDirectory(p)
	require.NoError(t, err)
	row, ok := directory.RowFor("https://witness.example")
	require.True(t, ok, "the pinned endpoint has a row, trailing slash or not")
	keys, err := row.Keys()
	require.NoError(t, err)
	require.Len(t, keys, 1)
	assert.Equal(t, public, keys[0])
	_, ok = directory.RowFor("https://other.example")
	assert.False(t, ok, "no other witness is trusted")
}

func TestPinnedWitnessDirectoryOfAnUnpinnedProfileTrustsNoWitness(t *testing.T) {
	directory, err := pinnedWitnessDirectory(cli.Profile{})
	require.NoError(t, err)
	_, ok := directory.RowFor("https://witness.agentactioncapsule.org")
	assert.False(t, ok, "an unverified receipt must never read as witnessed (evidencebook#115)")
}
