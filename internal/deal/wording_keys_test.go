package deal

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/action-state-group/capsule-cli/cli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every wording key this plugin's code names is in the built-in packs this
// process speaks with (the core's and this plugin's own), so no line is left
// out for a missing key (Line drops one). capsule-cli's
// TestEveryWordingKeyTheCodeNamesIsInThePack checks the core's code; this is
// the same check over this plugin's code (capsule-cli#326, design note
// section 3.1; capsule-cli#428).
func TestEveryWordingKeyTheDealCodeNamesIsInThePack(t *testing.T) {
	w := cli.BuiltinWording()
	key := regexp.MustCompile(`"((?:say|card|report|action)\.[a-z_.]*[a-z_])"`)
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)
	checked := 0
	var missing []string
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		checked++
		src, err := os.ReadFile(f)
		require.NoError(t, err)
		for _, m := range key.FindAllStringSubmatch(string(src), -1) {
			if _, ok := w.Entries[m[1]]; !ok {
				missing = append(missing, f+": "+m[1])
			}
		}
	}
	require.Positive(t, checked, "the glob matched no source file")
	assert.Empty(t, missing, "wording keys the built-in pack lacks")
}

// deal-view.js speaks only with the pack's page.* words (evidencebook-deal#97):
// every key it names is in the plugin's pack, and no page.* entry sits unused,
// so a word cannot be left out of the page for a missing key (W words nothing
// for one) and the pack holds no word the page never says. A key built at run
// time is named by its prefix, which ends in a dot.
func TestTheDealViewSpeaksWithThePackPageWords(t *testing.T) {
	entries := cli.BuiltinWording().Entries
	named := regexp.MustCompile(`"(page\.[a-z_0-9.]*)"`).FindAllStringSubmatch(DealViewJS, -1)
	require.NotEmpty(t, named)
	var prefixes []string
	exact := map[string]bool{}
	for _, m := range named {
		if strings.HasSuffix(m[1], ".") {
			prefixes = append(prefixes, m[1])
			continue
		}
		exact[m[1]] = true
		assert.Contains(t, entries, m[1], "deal-view.js names a page word the pack lacks")
	}
	for k := range entries {
		if !strings.HasPrefix(k, pagePrefix) {
			continue
		}
		used := exact[k]
		for _, p := range prefixes {
			used = used || strings.HasPrefix(k, p)
		}
		assert.True(t, used, "the pack's page word %s is never said by deal-view.js", k)
	}
}

// dealPageWordsEnv is the environment a node driver of deal-view.js runs
// with: the words a deal page carries (dealPageWords), which the driver
// puts on capsulectlDealPage as the page's bootstrap does.
func dealPageWordsEnv(t *testing.T) []string {
	t.Helper()
	raw, err := json.Marshal(dealPageWords(cli.BuiltinWording()))
	require.NoError(t, err)
	return append(os.Environ(), "DEAL_PAGE_WORDS="+string(raw))
}
