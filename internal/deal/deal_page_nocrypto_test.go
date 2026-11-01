package deal

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/action-state-group/capsule-cli/cli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// dealPageNoCryptoDriver runs a deal page's own runtime and deal view in a
// vm with no `crypto` global, as a page opened from a file: URL or plain
// http has none (WebCrypto exists only in a secure context), and prints
// what the runtime made of the bundle.
const dealPageNoCryptoDriver = `
const fs = require("fs");
const vm = require("vm");
const withCrypto = process.argv[3] === "crypto";
const html = fs.readFileSync(process.argv[2], "utf8");
const scripts = [...html.matchAll(/<script\b[^>]*>([\s\S]*?)<\/script>/g)].map((m) => m[1]);
const bundleJSON = scripts.find((s) => html.slice(0, html.indexOf(s)).endsWith('<script type="application/json">'));
const runtime = scripts.find((s) => s.includes("var EvidenceGraph = "));
const modules = scripts.filter((s) => s.includes("registerPresentation") && s !== runtime);
const sandbox = { console, TextEncoder, TextDecoder, atob, btoa, URL, setTimeout, clearTimeout, queueMicrotask };
if (withCrypto) sandbox.crypto = require("crypto").webcrypto;
const context = vm.createContext(sandbox);
vm.runInContext("globalThis.window = globalThis;", context);
vm.runInContext(runtime, context);
for (const source of modules) vm.runInContext(source, context);
const EG = vm.runInContext("EvidenceGraph", context);
(async () => {
  const out = { crypto: vm.runInContext("typeof crypto", context), ed25519: EG.ed25519Available() };
  const ctx = await EG.buildVerifiedBundleContext(vm.runInContext("JSON.parse(" + JSON.stringify(bundleJSON) + ")", context));
  const v = ctx.verification;
  out.checks = { graphClosure: v.graphClosure.status, intervalCoverage: v.intervalCoverage.status, perRecordMembership: v.perRecordMembership.status };
  out.records = Object.values(v.capsuleResults).map((r) => r.ok);
  out.findings = Object.values(v.capsuleResults).flatMap((r) => r.findings.map((f) => f.code));
  out.disclosures = v.disclosures.map((d) => d.status);
  const resolution = await EG.resolvePresentation(ctx);
  out.presentation = resolution.kind;
  if (resolution.kind === "module") {
    const view = await resolution.module.buildModel(ctx);
    out.refusal = view.refusal || "";
    out.scope = view.report ? view.report.scope : "";
  }
  process.stdout.write(JSON.stringify(out));
})().catch((e) => { process.stdout.write(JSON.stringify({ error: String(e && e.stack || e) })); });
`

// A deal page opened where WebCrypto is missing (a file: or plain http page is
// not a secure context) must not read a valid bundle as did-not-verify. The
// runtime checks the records, graph and coverage with its plain SHA-256; the
// deal view cannot check the report's Ed25519 signature there, so it says so
// and shows none of the report. With WebCrypto the same page shows it.
// (capsule-viewer/go v0.9.0 exports no switch to turn the SHA-256 fallback
// off, so the "unable to verify in this viewer" page is not reachable here.)
func TestADealPageWithoutWebCrypto(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	dealFixture(t)
	id := retailDeal(t)
	dir := t.TempDir()
	page := filepath.Join(dir, "shared.html")
	dealRun(t, "report", "--deal", id, "--share", cli.DealAudienceCounterparty, "--to", "the shop", "--html", page)
	driver := filepath.Join(dir, "driver.js")
	require.NoError(t, os.WriteFile(driver, []byte(dealPageNoCryptoDriver), 0o600))
	run := func(mode string) map[string]any {
		out, err := exec.Command(node, driver, page, mode).CombinedOutput()
		require.NoError(t, err, string(out))
		var got map[string]any
		require.NoError(t, json.Unmarshal(out, &got), string(out))
		require.NotContains(t, got, "error")
		return got
	}

	bare := run("bare")
	assert.Equal(t, "undefined", bare["crypto"])
	assert.Equal(t, false, bare["ed25519"])
	assert.Equal(t, map[string]any{"graphClosure": "pass", "intervalCoverage": "pass", "perRecordMembership": "pass"}, bare["checks"])
	assert.NotEmpty(t, bare["records"])
	for _, ok := range bare["records"].([]any) {
		assert.Equal(t, true, ok, "every record verifies with the plain SHA-256")
	}
	assert.Equal(t, "module", bare["presentation"])
	assert.Equal(t, "⚠️ This viewer cannot check the report's signature here. Run capsulectl verify on the file before relying on it.", bare["refusal"])
	assert.Empty(t, bare["scope"], "no report text is shown unchecked")

	secure := run("crypto")
	assert.Equal(t, true, secure["ed25519"])
	assert.Empty(t, secure["refusal"])
	assert.NotEmpty(t, secure["scope"])
}
