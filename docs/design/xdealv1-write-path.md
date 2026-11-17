# Design note: the plugin's x-deal-v1 write path

Issue: evidencebook-deal#212 (evidencebook#461 step 5, the plugin half).
Status: round 1, for review by Codex, Kimi and Sherlock2 (it adds user-facing verbs and changes
what two existing verbs do on a new deal).
Checked on evidencebook main at `ae7e898` (x-deal-v1 records #487, sent records #488, library
calls #484/#486) and this repository at `d17d543`. The wire shapes are evidencebook's
`docs/design/461-deal-profile.md` (§3.1 to §3.5, §9); this note does not restate them.

Each rule is marked **Decided** (a source is named) or **Proposed** (this note's choice).

## 1. What this note covers

The part of #212 that lets the plugin write and send v1 records: version selection, the
deal-book writer, the journal writer, seal-and-send, receiving, and what `deal close` and
`deal terms-result` do on a v1 deal. Not covered, because the library parts are not on main yet
(`Start`, `StatusProvider`, `ReceiveProfile`, `PaymentCheck`, `CanAct`, the v1 report, evidencebook
PRs 3 to 5): row status in `deal report`/`list`/`settlement`, the stamp-proof order, the v1
report, and `standard_history`. A later note covers them.

## 2. What exists

- The deal engine is a v0 event fold: `dealEvent`, `foldDeal`, `sealedEvent` (`deal.go`), sealed
  as one `x-deal-v0` record per step through `sealBookStep` (`deal_books.go:612`) onto the deal's
  book (`s.book`), whose header comes from `dealprofile.StepCapsule`.
- A deal's layout is chosen per deal in `useDeal` (`deal.go:211-263`): legacy SQLite index, or a
  book; `shelfDealIndex` (`deal_books.go:50`) answers reads for both.
- Nothing sends a record to the other side. A copy is a file (`deal report --share`,
  `disclose --out`), and the counterparty verifies it with `verify`.
- The shelf already has `SealSent`, `Receive`, `CountedPrefix`, `Stamp` and `HeldFirst`
  (evidencebook `shelf_sent.go:47`, `shelf_held.go:25`).

## 3. Version selection

**Decided** (evidencebook design §2.3): a record's version is the one `x-deal-v<N>` member it
carries (`dealprofile.Version`), a book has the version of its first record, a deal has one
version on both sides, and the offer's version decides.
**Decided** (Q7, coordinator, 2026-11-17): v0 stays the default until `/3` is served; v1 is
selectable by setting.

**Proposed:**

1. The setting is the profile's `deal_profile` value, `x-deal-v0` (default) or `x-deal-v1`, set
   once with `capsulectl deal declare --profile x-deal-v1` beside the existing `declare`, and read
   at a new deal's start. A deal already open keeps its version: it is read from the deal's first
   record, never from the setting, so changing the setting never touches an open deal.
2. `useDeal` gains one branch after the legacy test: a book whose first record decodes as
   `x-deal-v1` is a v1 deal (`s.version == "x-deal-v1"`). Every verb that is v0-only refuses a v1
   deal with the message of section 7; every v1 verb refuses a v0 deal the same way, naming the
   deal's version.
3. A v1 deal is a book on the shelf (`deal/<deal_id>`), never in the legacy SQLite index
   (`isLegacy` is false: there are no `deal_steps` rows).

## 4. The writers

### 4.1 The deal-book writer

**Decided** (design §3.1): a deal-book record is appended through `Book.Append`/`AppendCapsule`
with `record_type` `deal_step`, `epistemic_type` `producer_claim`, the record digest as its one
payload commitment, one `cites` link to the previous record id, and the block's `seq`, `prev`,
`agreement_ref`, `seen`.

**Proposed:** one function, `sealV1(ctx, dealID, rec xdealv1.Record) (sealed, error)` beside
`sealBookStep`, that fills the block's chain members (`seq`, `prev`, `agreement_ref`, `at`, and
`seen` from `Shelf.CountedPrefix(counterparty_key, book_log_id).Last` read at sealing), runs
`xdealv1.Check` (and `CheckSealed` for a journal record) before anything is stored, and appends
through the existing `appendTo`. It reuses `appendTo`'s signing, crash hooks and `inFlight`. It
does not reuse the v0 `fixStep`, `stepRequest` or `checkSealable`.

### 4.2 The journal writer and seal-and-send

**Decided** (design §3.1, evidencebook `Shelf.SealSent`): a journal record that leaves the shelf
whole (offer, decline, cancellation) is sealed twice in one call, as a journal record and as a
standalone Capsule under `action_id` `<deal_id>/sent`. A journal record that does not leave
(intent, check, verdict, approval, admit, the journey and the sale overlays) is appended to the
journal book with `CheckSealed`.

**Proposed:** `sealJournalV1` appends to `shelf.Journal(ctx)`; `sealSentV1` calls `SealSent` and
writes the standalone Capsule (`xdealv1.Pair`) to the file the user names. On `SealSent`'s
partial-failure result (a Record with an error) the verb sends the returned Capsule and does not
seal again, as the library says. A write that fails after the journal record is stored is retried
by re-running the verb, which finds the record by its digest and writes the file again.

### 4.3 Sending and receiving

**Proposed:** the plugin has no transport and gains none here. "Sent when sealed" means the
standalone Capsule is written, as a file, at the moment the record is sealed (`--out FILE`, or
stdout), so the host (Goose, Muse) sends it by its own means. `deal receive FILE` reads one
standalone Capsule, runs `xdealv1.VerifySent` (with an `OfferContext` when the deal's offer is
held), then `Shelf.Receive`, and prints what the library says changed (the deal ids whose counted
prefix grew, shrank or forked). It seals nothing and appends nothing to a book.

## 5. The verbs

Every new verb is user-facing, so this section is the one Sherlock2 confirms. Names avoid
today's: `deal decline` already declines a held evidence request (`deal respond`'s held list), so
the v1 decline is a subcommand of `offer`.

| Verb | Writes | Where | Notes |
|---|---|---|---|
| `deal offer --input FILE --out FILE` | `propose` (offer or counter-offer, `--counter-to DIGEST`) | journal, sent | the input holds terms, rows, named records, pack digest, witnesses, adjudicator, `agree_by` |
| `deal offer decline OFFER_DIGEST --reason not_selected --out FILE` | `agree` kind `decline` | journal, sent | Sherlock2 2a004e65: the declined buyer's copy says so and verifies |
| `deal offer cancel ... --out FILE` | `agree` kind `cancellation` | journal, sent | before the deal starts (design §3.5) |
| `deal agree OFFER --deal ID` | `agree` kind `agreement` | deal book, `seq` 1 | opens the deal book, cites the intent and admission, checks `CheckAgreement` |
| `deal receive FILE` | nothing | the store | section 4.3 |
| `deal act`, `deal pay` | `perform`, `settle` | deal book | the v0 `note --action` forms, mapped; `--check DIGEST` cites the passing check |
| `deal statement --row ROW --value met\|not_met\|cannot_tell` | `statement` | deal book | |
| `deal dispute --row ROW`, `... --withdraw`, `... --challenge RULING` | `dispute` | deal book | `forum` from the agreement's adjudicator |
| `deal check`, `deal note` | the journal overlay and journey kinds | journal | unchanged flags; on a v1 deal they seal the v1 records |

The rows' vocabulary and the statement words are rowstatus's, used exactly (Sherlock2 2a004e65,
capsule-viewer#322 answer): the plugin adds no label of its own.

## 6. Settled and local

**Decided** (Sherlock2 2a004e65, design §4.2): where a v0 deal showed "closed", a v1 deal shows
only the computed **settled** ("every row is agreed, uncontested, or ruled with its remedy done")
or nothing; no new done or closed state. On a shelf with no witness a row status shows as
**local** (acceptance 3). Both come from `rowstatus` and `Start`, so this note's code only
passes them through once evidencebook PRs 3 to 5 land.

## 7. `deal close` and `deal terms-result` on a v1 deal

**Decided** (Sherlock2 2a004e65, evidencebook-deal#212 How ack): on a v1 deal they print a
message and exit non-zero without sealing: "this deal has no close; its rows are settled by
statements and dates: see `deal report` and `deal statement`" (wording is Sherlock2's call; this is
a placeholder until it confirms). `deal --help` and each command's help say which deals still
have them (v0 deals). A v0 deal keeps both verbs unchanged.

## 8. Old deals, the demo and the pin

- v0 deals, their close records and results logs, `standard_history/1` and `/2`, the temporary
  `outcome_note` verb and `examples/ard-commerce` are not changed. With the setting at its
  default, no code path of this note runs.
- The evidencebook pin moves only to a released version that contains `x-deal-v1`. Until then
  this work builds on a pseudo-version in a branch that is not merged. It also needs capsule-viewer
  to bundle cll 0.4.0 (capsule-viewer#325), because evidencebook main pulls cll v0.4.0 and the
  plugin's page tests fail without it.

## 9. Tests

- Round trips: each v1 record written by the verbs decodes, passes `Check`, and verifies under
  `capsulectl verify`.
- An offer, an agreement and a statement between two profiles (two shelves in one test), the
  offer carried as a file and received with `Receive`.
- A decline written, received and verified by the buyer's profile.
- Version isolation: with the default setting, the existing suite is unchanged; a v1 deal refuses
  `deal close`; a v0 deal refuses `deal statement`; changing the setting does not change an open
  deal.
- A crash after `SealSent` stored the journal record: re-running the verb writes the same file.

## 10. Open questions

1. **Verb names and input files** (section 5) are proposals; Sherlock2 confirms the names, and
   the offer input's field names are evidencebook's (§3.3).
2. **Where `deal agree` gets the admission and intent digests** it must cite (design §3.3,
   `admit`, `intent`): from the journal's own records. The plugin writes `admit` today only as v0
   `verdict`/`approval` steps; whether the existing admission flow produces a v1 `admit` is
   section 12 territory of evidencebook's note and is not settled here.
3. **The journal overlay for the sale flow** (`sale`, `thread`, `thread_opened`, `sale_unpost`,
   `counterparty_profile`, `disclosure`): sealed in the journal on a v1 deal (design §3.2). This
   note does not move the sale verbs; a following slice does.
