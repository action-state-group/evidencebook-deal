package deal

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/action-state-group/capsule-cli/cli"
	"github.com/action-state-group/evidencebook"
	"github.com/action-state-group/evidencebook/dealprofile/xdealv1"
	"github.com/spf13/cobra"
)

func dealReceiveCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "receive FILE",
		Short: "Take a file the other side sent: an offer, a decline, a cancellation, an agreement or a record, with its stamp proof",
		Long: `Take a file the other side sent and keep it in your deal store. FILE holds one
of: an offer, a decline or a cancellation (what 'deal offer' wrote), a deal-book
record such as an agreement, or a push {record, stamp_proof} (what 'deal agree'
writes for the acceptor), or a stamp proof alone. Each record is checked on its
own before it is kept, and a stamp proof against the records you hold.

It says in plain words what it did: which records are kept, whether one is
still missing before a counted one, and whether two different records came for
one place (it does not say which is right). It seals nothing and appends
nothing to a book of yours.

Needs the deal store in the new layout (deal set-version x-deal-v1 --migrate).`,
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			raw, err := cli.ReadInput(args[0])
			if err != nil {
				return err
			}
			var push v1Push
			var members map[string]json.RawMessage
			if err := json.Unmarshal(raw, &members); err != nil {
				return cli.InputError("the file is not a JSON object: " + err.Error())
			}
			_, hasProof := members["stamp_proof"]
			_, hasRecord := members["record"]
			if hasProof || (hasRecord && members["agent_input"] == nil) {
				if err := cli.DecodeJSONAs("the file", raw, &push); err != nil {
					return err
				}
			} else {
				push.Record = raw
			}
			v1ReceiveContext(c)
			return runDeal(c, false, func(ctx context.Context, s *dealSession, _ string, _ []sealedEvent) error {
				if _, err := s.requireMigrated(ctx); err != nil {
					return err
				}
				if err := s.ensureV1Tables(ctx); err != nil {
					return err
				}
				return s.receiveV1(ctx, c, push)
			})
		},
	}
	return cmd
}

// receivedInfo is what the plugin keeps of a record it verified before the
// store took it.
type receivedInfo struct {
	kind    string
	dealID  string
	sent    *xdealv1.Sent
	book    *xdealv1.BookRead
	payload []byte
}

// readReceived tells which form a record file is in and verifies it on its
// own: a sent journal record (an offer, a decline, a cancellation) or a
// deal-book record.
func (s *dealSession) readReceived(ctx context.Context, raw []byte) (*receivedInfo, error) {
	var members map[string]json.RawMessage
	if err := json.Unmarshal(raw, &members); err != nil {
		return nil, cli.InputError("the record is not a JSON object: " + err.Error())
	}
	switch {
	case members["agent_input"] != nil:
		first, err := xdealv1.VerifySent(raw, nil)
		if err != nil {
			return nil, cli.InputError("the sent record does not verify: " + err.Error())
		}
		// A decline or a cancellation is read against the offer it names when
		// the holder has it: its signer and its digest.
		if ag, ok := agreeOf(first.Record); ok {
			held, err := s.offerByDigest(ctx, ag.Offer.Digest)
			if err != nil {
				return nil, err
			}
			if held != nil {
				if first, err = xdealv1.VerifySent(raw, &xdealv1.OfferContext{Offer: held.Record, Signer: held.Signer}); err != nil {
					return nil, cli.InputError("the sent record does not verify against the offer it answers: " + err.Error())
				}
			}
		}
		return &receivedInfo{kind: first.Record.Block.RecordType, dealID: first.Record.Block.DealID, sent: &first}, nil
	case members["payload"] != nil:
		read, err := xdealv1.VerifyBook(raw)
		if err != nil {
			return nil, cli.InputError("the deal-book record does not verify: " + err.Error())
		}
		return &receivedInfo{kind: read.Record.Block.RecordType, dealID: read.Record.Block.DealID, book: &read}, nil
	}
	return nil, cli.InputError("the file holds no record this profile reads: expected a sent record (agent_input) or a deal-book record (payload)")
}

func (s *dealSession) receiveV1(ctx context.Context, c *cobra.Command, push v1Push) error {
	var info *receivedInfo
	if len(push.Record) > 0 {
		var err error
		if info, err = s.readReceived(ctx, push.Record); err != nil {
			return err
		}
		if err = s.refuseOwnDeal(ctx, info); err != nil {
			return err
		}
	}
	if info == nil && push.StampProof == nil {
		return cli.InputError("the file holds neither a record nor a stamp proof")
	}
	res, rerr := s.shelf.Receive(ctx, push.Record, push.StampProof)
	out := map[string]any{}
	if info != nil {
		out["record_type"], out["deal_id"] = info.kind, info.dealID
	}
	if res.Record != nil {
		out["record"] = recordWords(res.Record)
		if res.Record.Stored || res.Record.Duplicate {
			if err := s.noteReceived(ctx, info); err != nil {
				return errors.Join(rerr, err)
			}
		}
	}
	if res.Proof != nil {
		out["stamp_proof"] = proofWords(res.Proof)
	}
	if rerr != nil {
		out["refused"] = rerr.Error()
		_ = cli.Output(c, out)
		return rerr
	}
	return cli.Output(c, out)
}

// refuseOwnDeal refuses a record that claims a deal this profile made the
// offer of, from anyone but the other side's own book: an offer for a deal id
// this profile already holds is never taken over.
func (s *dealSession) refuseOwnDeal(ctx context.Context, info *receivedInfo) error {
	if info.sent == nil || info.kind != xdealv1.TypePropose {
		return nil
	}
	d, err := s.v1DealOf(ctx, info.dealID)
	if err != nil {
		return err
	}
	if d != nil {
		held, err := s.offerByDigest(ctx, info.sent.Digest)
		if err != nil {
			return err
		}
		if held == nil || held.Own {
			return cli.InputError("this profile already holds deal " + info.dealID + ": an offer for a deal id it holds is refused")
		}
	}
	return nil
}

func recordWords(o *evidencebook.RecordOutcome) string {
	switch {
	case o.Refused != nil:
		return "refused: " + o.Refused.Error()
	case o.Duplicate:
		return "already held"
	case o.Deleted:
		return "held once and since deleted under your retention"
	case o.Stored && o.Counted:
		return "kept, and counted"
	case o.Stored && o.AskForBook:
		return "kept, but a record before it is missing: ask the sender for its book"
	case o.Stored:
		return "kept"
	}
	return "not kept"
}

func proofWords(o *evidencebook.ProofOutcome) string {
	switch {
	case o.Refused != nil:
		return "refused: " + o.Refused.Error()
	case o.Duplicate:
		return "already held"
	case o.Accepted:
		return "kept"
	}
	return "not kept"
}

// noteReceived records what the plugin needs of a record the store took: the
// offer's payload to answer it, the deal's row, the other side's book and its
// agreement.
func (s *dealSession) noteReceived(ctx context.Context, info *receivedInfo) error {
	if info == nil {
		return nil
	}
	switch {
	case info.sent != nil && info.kind == xdealv1.TypePropose:
		prop, _ := proposeOf(info.sent.Record)
		payload, digest, err := xdealv1.Encode(info.sent.Record)
		if err != nil {
			return err
		}
		if _, err = s.db.ExecContext(ctx, `INSERT INTO deal_versions (deal_id, version, own_log_id, counterparty_log_id, state) VALUES (?, ?, '', '', 'offer_received') ON CONFLICT(deal_id) DO NOTHING`, info.dealID, dealVersionV1); err != nil {
			return err
		}
		if _, err = s.db.ExecContext(ctx, `INSERT INTO deal_v1 (deal_id, side, role, offer_digest, counterparty_key, counterparty_agreement, own_agreement_id, own_agreement_digest) VALUES (?, ?, ?, ?, ?, '', '', '') ON CONFLICT(deal_id) DO NOTHING`,
			info.dealID, xdealv1.SideAcceptor, oppositeRole(prop.Role), digest, info.sent.KeyID); err != nil {
			return err
		}
		_, err = s.db.ExecContext(ctx, `INSERT INTO deal_v1_offers (offer_digest, deal_id, signer_key, payload) VALUES (?, ?, ?, ?) ON CONFLICT(offer_digest) DO NOTHING`, digest, info.dealID, info.sent.KeyID, payload)
		return err
	case info.sent != nil:
		state := "declined"
		if ag, ok := agreeOf(info.sent.Record); ok && ag.Kind == xdealv1.KindCancellation {
			state = "cancelled"
		}
		_, err := s.db.ExecContext(ctx, `UPDATE deal_versions SET state=? WHERE deal_id=? AND state IN ('offered','offer_received')`, state, info.dealID)
		return err
	case info.book != nil:
		d, err := s.v1DealOf(ctx, info.dealID)
		if err != nil || d == nil {
			return err
		}
		if d.CounterpartyLogID == "" {
			if _, err = s.db.ExecContext(ctx, `UPDATE deal_versions SET counterparty_log_id=? WHERE deal_id=?`, info.book.LogID, info.dealID); err != nil {
				return err
			}
		}
		if d.CounterpartyKey == "" || d.CounterpartyKey == info.book.KeyID {
			if _, err = s.db.ExecContext(ctx, `UPDATE deal_v1 SET counterparty_key=? WHERE deal_id=?`, info.book.KeyID, info.dealID); err != nil {
				return err
			}
		}
		if ag, ok := agreeOf(info.book.Record); ok && ag.Kind == xdealv1.KindAgreement && info.book.Seq == 1 && info.book.LogID == firstNonEmpty(d.CounterpartyLogID, info.book.LogID) {
			_, err = s.db.ExecContext(ctx, `UPDATE deal_v1 SET counterparty_agreement=? WHERE deal_id=? AND counterparty_agreement=''`, info.book.RecordID, info.dealID)
		}
		return err
	}
	return nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
