package deal

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/action-state-group/agent-action-capsule/go/canonical"
	"github.com/action-state-group/capsule-cli/cli"
	emit "github.com/action-state-group/capsule-emit/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The baseline vectors freeze what today's builders make of real deals, so
// the move of the x-deal-v0 profile into evidencebook (evidencebook#93) can
// show it changes no byte: for each step, the record's payload and digest,
// the step's Capsule as sealed before any book header is added, and the
// header fields the book commits; for each sealed report, its payload and
// Capsule. Each vector also carries the inputs today's builders took (the
// deal's local events, its deal key and the profile's signing seed, all
// test-only values), so this test re-derives every output from them.
// DEAL_WRITE_BASELINE_VECTORS=1 regenerates the file from the flows below.
const baselineVectorsPath = "testdata/baseline-vectors.json"

type baselineVectors struct {
	Note  string         `json:"note"`
	Deals []baselineDeal `json:"deals"`
}

type baselineDeal struct {
	Flow        string           `json:"flow"`
	DealID      string           `json:"deal_id"`
	DealKey     string           `json:"deal_key"`
	SigningSeed string           `json:"signing_seed"`
	Operator    string           `json:"operator"`
	Events      []baselineEvent  `json:"events"`
	Steps       []baselineStep   `json:"steps"`
	Reports     []baselineReport `json:"reports,omitempty"`
}

// baselineEvent is a sealed step as the deal store holds it: its local
// event (the builders' input) and what it was sealed as.
type baselineEvent struct {
	CapsuleID string          `json:"capsule_id"`
	Digest    string          `json:"digest"`
	Sequence  uint64          `json:"sequence"`
	Event     json.RawMessage `json:"event"`
}

type baselineStep struct {
	N         int64          `json:"n"`
	Payload   string         `json:"payload"`
	Digest    string         `json:"digest"`
	CapsuleID string         `json:"capsule_id"`
	Capsule   string         `json:"capsule"`
	Header    baselineHeader `json:"header"`
}

type baselineHeader struct {
	RecordType string            `json:"record_type"`
	SubjectRef string            `json:"subject_ref"`
	Links      []string          `json:"cites,omitempty"`
	Fields     map[string]string `json:"fields"`
}

type baselineReport struct {
	Form       string `json:"form"`
	Timestamp  string `json:"timestamp"`
	AgentInput string `json:"agent_input"`
	Payload    string `json:"payload"`
	CapsuleID  string `json:"capsule_id"`
	Capsule    string `json:"capsule"`
}

func TestBaselineVectors(t *testing.T) {
	if os.Getenv("DEAL_WRITE_BASELINE_VECTORS") == "1" {
		writeBaselineVectors(t)
	}
	raw, err := os.ReadFile(baselineVectorsPath)
	require.NoError(t, err)
	var v baselineVectors
	require.NoError(t, json.Unmarshal(raw, &v))
	require.NotEmpty(t, v.Deals)
	for _, d := range v.Deals {
		t.Run(d.Flow, func(t *testing.T) {
			s := baselineSession(t, d)
			events := make([]sealedEvent, 0, len(d.Events))
			for _, e := range d.Events {
				var ev dealEvent
				require.NoError(t, cli.DecodeJSON(e.Event, &ev))
				events = append(events, sealedEvent{CapsuleID: e.CapsuleID, Digest: e.Digest, Sequence: e.Sequence, Event: ev})
			}
			require.Len(t, d.Steps, len(events))
			for i, want := range d.Steps {
				got, err := baselineStepOf(s, events[:i], events[i].Event)
				require.NoError(t, err, "step %d", want.N)
				assert.Equal(t, want, got, "step %d", want.N)
			}
			for _, want := range d.Reports {
				at, err := time.Parse(time.RFC3339, want.Timestamp)
				require.NoError(t, err)
				got, err := baselineReportOf(s, want.Form, at, want.AgentInput)
				require.NoError(t, err)
				assert.Equal(t, want, got, "%s report", want.Form)
			}
		})
	}
}

// baselineSession is the part of a deal session the step and report
// builders read: the profile's name and signing key, and the deal's key.
func baselineSession(t *testing.T, d baselineDeal) *dealSession {
	t.Helper()
	seed, err := hex.DecodeString(d.SigningSeed)
	require.NoError(t, err)
	dkey, err := hex.DecodeString(d.DealKey)
	require.NoError(t, err)
	return &dealSession{p: cli.Profile{Name: d.Operator}, key: ed25519.NewKeyFromSeed(seed), dkey: dkey}
}

// baselineStepOf derives a step's vector as sealBookStep builds it, up to
// the book's own header extension.
func baselineStepOf(s *dealSession, events []sealedEvent, ev dealEvent) (baselineStep, error) {
	request, digest, err := s.stepRequest(events, ev, dealCapsuleCurrent)
	if err != nil {
		return baselineStep{}, err
	}
	record, err := cli.Seal(request, s.key)
	if err != nil {
		return baselineStep{}, err
	}
	local, err := json.Marshal(ev)
	if err != nil {
		return baselineStep{}, err
	}
	amount, currency, err := closedTerms(events, ev)
	if err != nil {
		return baselineStep{}, err
	}
	recordType, _ := dealStepHeader(ev.DealID, ev)
	header := baselineHeader{
		RecordType: recordType, SubjectRef: request.Capsule.ActionID,
		Fields: map[string]string{dealLocalField: string(local), dealAmountField: amount, dealCurrencyField: currency},
	}
	if parent := request.Capsule.Chain; parent != nil {
		header.Links = []string{parent.ParentCapsuleID}
	}
	return baselineStep{
		N: ev.N, Payload: string(request.Payload), Digest: digest,
		CapsuleID: record.CapsuleID, Capsule: string(record.Capsule), Header: header,
	}, nil
}

// baselineReportOf derives a report's vector as dealSealReport and
// dealStandaloneReport build it, from the report's input and time.
func baselineReportOf(s *dealSession, form string, at time.Time, input string) (baselineReport, error) {
	generic, err := cli.DecodeBundleJSON([]byte(input))
	if err != nil {
		return baselineReport{}, err
	}
	payload, err := canonical.JCS(generic)
	if err != nil {
		return baselineReport{}, err
	}
	record, err := cli.Seal(cli.Request{
		Version: "capsule-seal-request/v1",
		Capsule: emit.Input{
			ActionID: cli.DealReportActionID, ActionType: emit.ActionTypeFYI, Operator: s.p.Name, Developer: "capsulectl-deal",
			Timestamp: at.UTC(),
		},
		Payload: payload,
	}, s.key)
	if err != nil {
		return baselineReport{}, err
	}
	return baselineReport{
		Form: form, Timestamp: at.UTC().Format(time.RFC3339), AgentInput: input,
		Payload: string(payload), CapsuleID: record.CapsuleID, Capsule: string(record.Capsule),
	}, nil
}

// baselineFlows are the deals the vectors are made from: most record types,
// x-deal-v0 and typed records, a buyer and a seller, and a report off the
// deal's log and one in it.
var baselineFlows = []struct {
	name  string
	inLog bool
	run   func(t *testing.T) string
}{
	{"retail-closed", false, func(t *testing.T) string {
		dealFixture(t)
		id := retailDeal(t)
		dealRun(t, "close", "--deal", id, "--input", writeJSON(t, `{"status":"received"}`))
		dealRun(t, "note", "--deal", id, "--kind", "evidence", "--input", writeJSON(t, `{"about":"the delivery","source":"agent"}`))
		return id
	}},
	{"retail-report-in-log", true, func(t *testing.T) string {
		dealFixture(t)
		return retailDeal(t)
	}},
	{"claims", false, claimDeal},
	{"typed-buyer", false, func(t *testing.T) string {
		id := stickerDeal(t, "card", true)
		dealRun(t, "check", "--deal", id, "--input", writeJSON(t, payCheck("card", 454, 500)))
		return id
	}},
	{"typed-seller-settled", false, func(t *testing.T) string {
		id, dueStep := sellerDealWithDueDelivery(t)
		terms, _ := newBuyerSide(t).terms()
		notePaymentReceived(t, id, terms, x402Settle(true))
		dealRun(t, "note", "--deal", id, "--kind", "delivered", "--terms", terms,
			"--content", writeFile(t, "content.bin", deliveredContent), "--resolves", dueStep)
		return id
	}},
}

func writeBaselineVectors(t *testing.T) {
	v := baselineVectors{Note: "Generated by TestBaselineVectors (DEAL_WRITE_BASELINE_VECTORS=1) from today's builders; every key and secret here is a test-only value."}
	for _, f := range baselineFlows {
		t.Run("write-"+f.name, func(t *testing.T) {
			old := dealReportsOffLog
			dealReportsOffLog = !f.inLog
			t.Cleanup(func() { dealReportsOffLog = old })
			id := f.run(t)
			bundle := filepath.Join(t.TempDir(), "bundle.json")
			dealRun(t, "report", "--deal", id, "--bundle", bundle)
			v.Deals = append(v.Deals, baselineDealOf(t, f.name, id, f.inLog, bundle))
		})
	}
	out, err := json.MarshalIndent(v, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(baselineVectorsPath, append(out, '\n'), 0o644))
}

func baselineDealOf(t *testing.T, flow, id string, inLog bool, bundlePath string) baselineDeal {
	t.Helper()
	p, err := cli.LoadProfile("deal")
	require.NoError(t, err)
	s, err := openDealSession(t.Context(), p)
	require.NoError(t, err)
	defer func() { require.NoError(t, s.close()) }()
	require.NoError(t, s.useDeal(t.Context(), id, false))
	events, err := s.load(t.Context(), id)
	require.NoError(t, err)
	d := baselineDeal{
		Flow: flow, DealID: id, DealKey: hex.EncodeToString(s.dkey),
		SigningSeed: hex.EncodeToString(s.key.Seed()), Operator: s.p.Name,
	}
	for i, se := range events {
		local, err := json.Marshal(se.Event)
		require.NoError(t, err)
		d.Events = append(d.Events, baselineEvent{CapsuleID: se.CapsuleID, Digest: se.Digest, Sequence: se.Sequence, Event: local})
		step, err := baselineStepOf(s, events[:i], se.Event)
		require.NoError(t, err)
		require.Equal(t, se.Digest, step.Digest, "the stored digest re-derives")
		d.Steps = append(d.Steps, step)
	}
	raw, err := os.ReadFile(bundlePath)
	require.NoError(t, err)
	b, err := cli.DecodeBundleJSON(raw)
	require.NoError(t, err)
	report, capsuleID, form := cli.DealSealedReport(b)
	require.NotNil(t, report, "the copy carries its sealed report")
	want := "standalone"
	if inLog {
		want = "in_log"
	}
	require.Equal(t, want, form)
	input, at := baselineReportInput(t, b, capsuleID)
	r, err := baselineReportOf(s, form, at, input)
	require.NoError(t, err)
	if !inLog {
		// Off the log, the copy's report is exactly the Capsule rebuilt here;
		// in it, the book adds its header, so only the payload is shared.
		require.Equal(t, capsuleID, r.CapsuleID)
	}
	d.Reports = append(d.Reports, r)
	return d
}

// baselineReportInput is the sealed report's disclosed input and its
// Capsule's time, read from the copy that carries it.
func baselineReportInput(t *testing.T, b map[string]interface{}, capsuleID string) (string, time.Time) {
	t.Helper()
	exts, _ := b["extensions"].(map[string]interface{})
	var input interface{}
	if section, ok := exts[cli.DealReportExtension].(map[string]interface{}); ok {
		input = section["agent_input"]
	} else {
		disclosures, _ := b["disclosures"].(map[string]interface{})
		d, _ := disclosures[capsuleID].(map[string]interface{})
		input = d["agent_input"]
	}
	require.NotNil(t, input, "the report's input is disclosed")
	capsule := findCapsule(b, capsuleID)
	require.NotNil(t, capsule, "the copy carries the report's Capsule")
	at, err := time.Parse(time.RFC3339, capsule["timestamp"].(string))
	require.NoError(t, err)
	raw, err := json.Marshal(input)
	require.NoError(t, err)
	return string(raw), at
}

// findCapsule is the first object under v whose capsule_id is id and that
// carries a timestamp: the Capsule itself, wherever the copy holds it.
func findCapsule(v interface{}, id string) map[string]interface{} {
	switch x := v.(type) {
	case map[string]interface{}:
		if x["capsule_id"] == id {
			if _, ok := x["timestamp"].(string); ok {
				return x
			}
		}
		for _, e := range x {
			if c := findCapsule(e, id); c != nil {
				return c
			}
		}
	case []interface{}:
		for _, e := range x {
			if c := findCapsule(e, id); c != nil {
				return c
			}
		}
	}
	return nil
}
