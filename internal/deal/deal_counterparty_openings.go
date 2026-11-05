package deal

import (
	"encoding/hex"
	"encoding/json"

	"github.com/action-state-group/evidencebook/dealprofile"
)

// The user's own copy opens who the deal was with and what the buyer wrote
// (evidencebook-deal#56). The owner already knows both, so a copy that stays
// with them tells no new party anything; a shared copy carries neither, and
// not the deal key either, which would let a holder test a guessed name
// against counterparty.ids.name.
//
// counterparty_openings holds the deal's key and, per step whose sealed
// record carries a keyed counterparty fingerprint, the raw name and domain
// that fingerprint was made from. acceptance_openings holds, per acceptance,
// its salted words (content_commitment). Both are left out when the deal has
// none, so a deal sealed before this change reports as it did.

// counterpartyOpeningKinds are the fingerprints a page may open.
var counterpartyOpeningKinds = []string{"name", "domain"}

// eventWhos are every counterparty identity a step names.
func eventWhos(ev dealEvent) []dealWho {
	var out []dealWho
	add := func(w *dealWho) {
		if w != nil {
			out = append(out, *w)
		}
	}
	if ev.Open != nil {
		out = append(out, ev.Open.Who)
	}
	for _, in := range []*dealIntent{ev.Intent, ev.TaskAuthority} {
		if in != nil {
			add(in.Who)
		}
	}
	if ev.Message != nil {
		add(ev.Message.Who)
	}
	if ev.Evidence != nil {
		add(ev.Evidence.Who)
	}
	if ev.Change != nil {
		add(ev.Change.Who)
	}
	if ev.Snapshot != nil {
		add(ev.Snapshot.Who)
	}
	if ev.Disclosure != nil {
		add(ev.Disclosure.Who)
	}
	return out
}

func whoValue(w dealWho, kind string) string {
	if kind == "domain" {
		return w.Domain
	}
	return w.Name
}

// sealedCounterpartyIDs are the keyed fingerprints a record payload seals for
// its counterparty: in the x-deal-v0 block, or, in a typed record, its body.
func sealedCounterpartyIDs(payload []byte) map[string]interface{} {
	var record map[string]interface{}
	if json.Unmarshal(payload, &record) != nil {
		return nil
	}
	return recordCounterpartyIDs(record)
}

func recordCounterpartyIDs(record map[string]interface{}) map[string]interface{} {
	block, _ := record["x-deal-v0"].(map[string]interface{})
	body, _ := record["body"].(map[string]interface{})
	for _, holder := range []map[string]interface{}{block, body} {
		cp, _ := holder["counterparty"].(map[string]interface{})
		if ids, _ := cp["ids"].(map[string]interface{}); ids != nil {
			return ids
		}
	}
	return nil
}

// counterpartyOpenings opens the name and the domain each step's sealed
// counterparty fingerprints were made from, or nil when none were.
func counterpartyOpenings(key []byte, events []sealedEvent) map[string]interface{} {
	if len(key) == 0 {
		return nil
	}
	var whos []dealWho
	for _, se := range events {
		whos = append(whos, eventWhos(se.Event)...)
	}
	steps := []interface{}{}
	var before []sealedEvent
	for _, se := range events {
		payload, _, err := encodeDealRecord(se.Event, before, key)
		before = append(before, se)
		if err != nil {
			continue
		}
		ids := sealedCounterpartyIDs(payload)
		entry := map[string]interface{}{"step": se.CapsuleID}
		for _, kind := range counterpartyOpeningKinds {
			want, _ := ids[kind].(string)
			if want == "" {
				continue
			}
			for _, w := range whos {
				value := whoValue(w, kind)
				if got, err := dealprofile.Fingerprint(key, kind, value); err == nil && got == want {
					entry[kind] = value
					break
				}
			}
		}
		if len(entry) > 1 {
			steps = append(steps, entry)
		}
	}
	if len(steps) == 0 {
		return nil
	}
	return map[string]interface{}{"key": hex.EncodeToString(key), "steps": steps}
}

// acceptanceOpenings open each acceptance's content_commitment: the step, and
// the buyer's words under the nonce they were committed with.
func acceptanceOpenings(events []sealedEvent) []interface{} {
	var out []interface{}
	for _, se := range events {
		a := se.Event.Acceptance
		if a == nil || se.Event.Nonces["accepted_words"] == "" {
			continue
		}
		out = append(out, map[string]interface{}{"step": se.CapsuleID, "nonce": se.Event.Nonces["accepted_words"], "text": a.Words})
	}
	return out
}
