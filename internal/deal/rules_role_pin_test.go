package deal

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// roleCheck runs one check on a deal under the profile's current pin and
// returns the check and the rules_pin its checker was sent ("" when none).
func roleCheck(t *testing.T, dealID, check, checker string) (map[string]any, any) {
	t.Helper()
	_ = os.Remove(checker + ".input")
	out := dealRun(t, "check", "--deal", dealID, "--input", writeJSON(t, check))
	raw, err := os.ReadFile(checker + ".input")
	require.NoError(t, err, "the checker ran")
	var input map[string]any
	require.NoError(t, json.Unmarshal(raw, &input))
	return out, input["rules_pin"]
}

// The rules checker is held to the acting role's pin alone (capsule-cli#323):
// swapping one role's pin by a profile update changes only that role's check,
// and the checker is sent only that role's pack and digests.
func TestARolePinChangesOnlyThatRolesCheck(t *testing.T) {
	buyer := stickerDeal(t, "card", false)
	seller := openTyped(t, sellerTyped)
	checker := stubChecker(t, "rules", checkerPrints("allow", passFinding))
	sellerCheck := `{"action":"offer","amount_minor":190000,"terms":{"item":"example bicycle","quantity":1,"price_minor":190000}}`
	pin := func(sellerDigest string) {
		pinChecker(t, map[string]any{"command": []string{checker}, "definition_digest": exampleRulesDigest,
			"roles": map[string]any{"buyer": map[string]any{"pack": "asg/everyday/buyer", "definition_digest": exampleRulesDigest},
				"seller": map[string]any{"pack": "asg/everyday/seller", "definition_digest": sellerDigest}}})
	}

	// The seller's pin names another ruleset than the checker reports: only
	// the seller's check stops.
	pin(secondRulesDigest)
	got, sent := roleCheck(t, buyer, payCheck("card", 454, 454), checker)
	assert.NotContains(t, differenceTexts(got), "the ruleset changed", "the buyer's pin is untouched")
	assert.Equal(t, map[string]any{"pack": "asg/everyday/buyer", "definition_digests": []any{exampleRulesDigest}}, sent, "the buyer's checker is sent the buyer's pin alone")
	got, sent = roleCheck(t, seller, sellerCheck, checker)
	assert.Equal(t, "pause", got["verdict"])
	assert.Contains(t, differenceTexts(got), "the ruleset changed since it was pinned")
	assert.Equal(t, map[string]any{"pack": "asg/everyday/seller", "definition_digests": []any{secondRulesDigest}}, sent)

	// Swapping the seller's pin back, and nothing else, lets its check run.
	pin(exampleRulesDigest)
	got, _ = roleCheck(t, seller, sellerCheck, checker)
	assert.NotContains(t, differenceTexts(got), "the ruleset changed")
}

// A profile without roles reads and checks as before: the flat digests, and no
// rules_pin the old checkers would not know.
func TestAProfileWithoutRolesSendsNoRolePin(t *testing.T) {
	id := stickerDeal(t, "card", false)
	checker := stubChecker(t, "rules", checkerPrints("allow", passFinding))
	pinChecker(t, map[string]any{"command": []string{checker}})
	_, sent := roleCheck(t, id, payCheck("card", 454, 454), checker)
	assert.Nil(t, sent)
	pinChecker(t, map[string]any{"command": []string{checker}, "definition_digest": secondRulesDigest})
	got, sent := roleCheck(t, id, payCheck("card", 454, 454), checker)
	assert.Contains(t, differenceTexts(got), "the ruleset changed since it was pinned")
	assert.Equal(t, map[string]any{"definition_digests": []any{secondRulesDigest}}, sent, "the flat pin applies to a role with no entry")
}
