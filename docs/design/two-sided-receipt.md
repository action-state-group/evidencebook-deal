# Design note: the receipt with the other party's half (9b frames 6 and 7)

Issues: evidencebook-deal#78 (a Mesh `x-mesh-*` half as the other party's copy), #79 (the
receipt read from a composition), #81 (a payer bundle with two rounds).
Follows evidencebook-deal#83 (0f873c1), which draws the receipt for one book.
Status: draft, for review by two Claude stand-ins and Sherlock2.

## Problem

Steven's 9b canvas draws the receipt in three cases: **stamped by both** (frame 6), **your
receipt only** (frame 7), and **no record** (named in the canvas's explainer). #83 draws the
receipt from one deal file, which holds one book. One book cannot say whether the other side
agrees, so that page says "Your copy only" and "Their side: not in this file". Sherlock2
accepted this as interim, not as the end state.

Frames 6 and 7 need the other party's half on the page, checked by the page. Sherlock2's final
answer on #78 (Buzz `069958ca`, with its conditions in `4835fefa`) sets four conditions:

- **C1.** The receipt keeps the same three cases and uses no Mesh wording. A missing field reads
  "not stated" and is never filled in.
- **C2.** "Stamped by both" shows only when the counterparty's half was received **and** its
  signature verifies. A half that is present but does not verify counts as "your receipt only".
- **C3.** The plugin builds the bundle, so the disclosure check runs on the node, not in the page.
- **C4.** An "as agreed" line about price or spend, or a "paid" line, goes to Steven if a Mesh
  record would be the source. Until he rules, the receipt shows neither from a Mesh record.

This note settles where the other half rides, how the page reads it, and what each line says.

## What exists

- **A container for both halves: AAC `composed/v1`.** See agent-disclosure-bundle-01,
  "The composed/v1 Extension". Each member is the outcome of one Evidence Request: `artifact`
  (a bundle, plus the signed response without the artifact bytes), `refusal` (the signed
  refusal) or `absence` (the requester's recorded absence, with its window). Joins declare that
  two members' root records concern the same thing. `pre_agreed_identifier` and
  `shared_artifact_digest` are derived; the other bases are reserved. `capsulectl verify` checks
  the block, including member closure and join states.
- **`capsulectl-deal deal request` and `deal compose`** (`internal/deal/deal_evidence.go`) already
  build `composed/v1` from recorded answers and absences. They join each pair of members on
  `/deal_id`. `deal compose --html --keep` joins the composer's own keep copy at the page, through
  the bootstrap. The keep copy is never a member. The page is the core's composed section, not
  deal-view.
- **The page runtime can verify a bundle inside a bundle.** `EvidenceGraph.buildVerifiedBundleContext`
  (capsule-viewer `ts/src/browser.ts`) is in the runtime's module contract "for each member of a
  composed/v1 bundle" (capsule-viewer#167).
- **Precedence.** The core's composed view, `aac.builtin.composed/v0`, is a *section*. It is
  resolved in its own registry after the page (capsule-viewer `go/presentation/builtin.go`), so a
  page module that requires `composed/v1` is not ambiguous with it.
- **Mesh** (Mesh-LLM/capsules-plugin at `mesh/main` 720a281, `crates/evidence-plugin/src/`):
  - Each node seals its own `x-mesh-poc-v1` half. The halves share the request
    (`effect.request_digest`, `agent_input_digest`) and `serving_provenance.exchange_id`.
  - Neither half cites the other: "each is sealed before its node can know the other's id"
    (`producer/capsule.rs` ~750). A provider's half names only the request.
  - A received half is held, not chained. The receiver seals a citing record
    (`received_half.cited_capsule_id`, `own_half_capsule_id`, `signature_ok`).
  - The payer's deal receipt (`deal_receipt.rs`) carries only its own `x-deal-v0` records. Its
    sealed report's `did[].steps` cite the payer's exchange capsule id (`deal_flow.rs` 812-870).
  - Asking for the other half (`ask_book.rs`) seals `evidence_request`, then
    `evidence_response` or `recorded_absence`. That is the same Evidence Request profile
    `deal request` uses.
- **Two rounds.** `x-deal-v0` PROFILE rule 7 lets an `open` close be followed by a later
  outcome, close and report. A later bundle carries every earlier report's Capsule with its
  payload withheld (PROFILE "Later bundles"). Mesh-LLM/capsules-plugin#169 (open) seals such a
  second round.

## Decision

### D1. The other half rides in `composed/v1`, read by deal-view

The receipt with both halves is a `composed/v1` bundle. deal-view registers a second page
manifest, `capsulectl.deal-view-composed/v0`, from the same script and stylesheet. It requires
`composed/v1` and forbids what the single-book manifest forbids. The single-book manifest adds
`composed/v1` to its `forbids.extensions`, so a composition that also carries `x-deal-v0`
resolves to one module. The core's composed section still renders below the page, unchanged, as
the raw view of the members.

The module renders only a deal composition: a root record sealed by `capsulectl-deal-composition`
whose disclosed input is `{type: deal_composition, profile: x-deal-v0}`, or the plugin's
equivalent (D4). For any other composition `canRender` is false and the page resolves as it
does today.

Rejected:

- **A new extension in the single-book bundle carrying the other half.** It changes the
  `dealprofile` wire shape (evidencebook) to say what `composed/v1` already says, and it would be
  a second container for both halves.
- **A Mesh receipt model in deal-view.** Sherlock2 withdrew route (b) and ruled out Mesh wording
  (C1).

### D2. Who "you" are, and which half is theirs

- **You** are the composer: the key that sealed the composition's root record.
- **Your half** is one of two things, in this order:
  1. The keep copy joined at the page, as `--keep` does today. The bootstrap carries the keep
     **bundle**, not lines. The page verifies it with `buildVerifiedBundleContext` and builds the
     #83 single-book model from it. It must hold under the same rules as a single-book page
     (`dealViewVerified`, the sealed or standalone report check). It must also carry audience
     `keep`, root key equal to the composer's key, and the composition's deal id. If any of these
     fails, the page is refused, as `privateHalfOf` refuses at build time.
  2. Without a keep copy, the composer's own `artifact` member (observer = composer), read as a
     shared copy, as #83 reads one. Without that either, "Your side" says it is not in this file.
- **Their half** is every member whose observer is not the composer. A composition of one deal
  has at most one other party, so more than one other observer is refused.

### D3. The three cases

Their half's state, from their member:

| Their member | Checked by the page | Case | Their side box |
|---|---|---|---|
| `artifact`, verifies (V1-V4 below) | yes | **Stamped by both** | amber, "Their side · in their book · as received" |
| `artifact`, any of V1-V4 fails | the failing check | **Your receipt only** | grey dashed; their record is in the file but did not check: "failed" on that stamp |
| `refusal` | the refusal's presence | **Your receipt only** | grey dashed; "They answered that they would not share their record, on DATE." |
| `absence` | the absence member | **Your receipt only** | grey dashed; "No record. Your agent asked and nothing came back by DATE; that is written down too." |
| none | n/a | **No record** | grey dashed; "No record. Nothing from their side is in this file." |

Their `artifact` member verifies when:

- **V1.** The member bundle verifies with the same strictness as `dealViewVerified`: closure,
  interval coverage, membership proven, every capsule OK, disclosures matched or withheld. The
  page reads this from the core's verification of the containing bundle, which verifies each
  carried member recursively (agent-action-capsule `ts/src/bundle.ts` ~1348,
  `verification.extensions["composed/v1"].composed`). It reads the member's records through
  `buildVerifiedBundleContext`.
- **V2.** Its root record's producer signature verifies (`verifyProducerEnvelope`) under a key
  equal to the member's declared observer. Its checkpoint's key is that observer's
  `custody_domain`. This is C2's "its signature verifies": the half's own signature, as sealed in
  their book.
- **V3.** The member carries its `response`, and the core's composed result reports it bound:
  digest `reproduced`, and no `response_request_digest_mismatch` or
  `response_artifact_digest_mismatch` finding. The response's own signature is a deployment
  signature that this verifier reports `signature_unverified` (`ts/src/composed.ts` 56, ~730).
  The page says so on the stamp and does not count it as checked. `deal compose` checked that
  signature when it recorded the answer.
- **V4.** It is about this deal. For an `x-deal-v0` copy, its root carries the composition's
  deal id, the same id your half carries. When your own member is in the block, the block's join
  between the two members derives `agree`. For a Mesh half, see D4.

The page still never says "stamped by both" from the block's join states alone. It re-derives
V1-V4 itself, and the block's declared states must agree with what it derives, or the page is
refused. A disagreement means the file says something its records do not.

The banner reads **"Stamped by both sides · checked"** only when their member verifies **and**
your half holds as #83 defines it. Otherwise the banner names the case: "Your receipt only ·
checked" or "No record · checked" when your half holds, and the "· not fully checked" form when it
does not. A single-book page (no composition) keeps #83's "Your copy only" and "Not in this file":
one file cannot know whether the other side keeps a record.

### D4. A Mesh half as their member (#78)

The plugin builds the composition (C3). This repository defines what deal-view reads, and the
plugin-side work is its own task in Mesh-LLM/capsules-plugin. The page reads it as follows:

- **Members.** Two `artifact` members, each a Mesh node's evidence bundle whose root is its own
  `x-mesh-poc-v1` half, proven under that node's signed checkpoint. One is the payer's own half
  (observer = composer). The other is the provider's, received by push or by ask. The provider's
  member may also be `refusal` or `absence` when the payer asked and got no half.
- **Join.** One `shared_artifact_digest` join between the two halves, with `pointer:
  /effect/request_digest`. Both halves carry the request's digest, and neither cites the other.
- **Your half** is the payer's keep copy (its `deal_receipt` bundle), joined at the page as in
  D2. It is tied to the composition by its sealed report: a `did[].steps` entry must cite the
  payer member's root capsule id. Without that citation the page is refused.
- **V4 for Mesh.** The join between the two halves derives `agree`, and the payer member's root is
  cited by your sealed report.
- **What their side box shows** comes only from the provider's sealed record, so C1 holds. Its
  time is the record's `timestamp`. "Stamped into their book" carries the time of their
  checkpoint. Their words are "not stated": Mesh seals no words. "As agreed" and "paid" lines
  never come from a Mesh record (C4).

### D5. The stamps

Each stamp says only what the page checked, in the five words. Words that differ from the frame
are marked ◇ for Sherlock2.

1. "Your yes is signed with your key" is unchanged from #83, read from your half.
2. "Each side's record is stamped into its own book and can't be changed after": **passed** when
   your half's records all matched (#83) and their member verified V1-V2. **missing** when there
   is no artifact member. **failed** when their member is present and V1 or V2 failed.
3. ◇ "Their record and yours are about the same deal" (`x-deal-v0`) or "...the same exchange"
   (Mesh): **passed** on V4.
   - The frame's "Their record points at yours, and yours at theirs" is not drawn. Neither
     profile's halves cite each other today: an `x-deal-v0` copy only names the deal id, and the
     Mesh halves cannot know each other's id.
   - A Mesh payer's citing record points one way only. It is not used for this stamp.
4. "The payment is confirmed by …": **not checked**, as in #83. The page checks no settlement,
   and "paid" from a Mesh record is Steven's (C4).
5. "An outside witness saw both books": **not checked** when either book carries a witness
   extension, since the page can't check a witness's signature; otherwise **missing**.

### D6. Two rounds (#81)

A two-round bundle is a later bundle under PROFILE "Later bundles":

- `extensions["x-deal-v0"].sealed_report` names the latest report, which is disclosed.
- Each earlier report's Capsule is present with its payload withheld.

deal-view already refuses only *disclosed* other reports (`otherReports`). Withheld ones are not
in `matched`, so the model should already read the latest report. This work adds a test that
shows it:

- A deal closed `open`, then a later outcome, close and report, cut into one bundle.
- The page shows the latest round's state.
- The earlier report is neither shown as missing evidence nor a refusal.

If the test shows a misreading, the fix goes in the model and gets its own test. No new copy is
expected. Any copy that turns out to be needed goes to Sherlock2.

## Producer side

- **capsulectl-deal.**
  - `deal compose --html` builds its page through deal-view's composed manifest. It keeps the
    core's composed section below the page.
  - `--keep` carries the keep bundle in the bootstrap, instead of its lines.
  - The composed bundle, its digest and every shared file stay byte-for-byte unchanged.
- **Mesh** (new task, Mesh-LLM/capsules-plugin): the composition in D4, sealed under the payer
  node's key, with the node's disclosure policy toward the provider applied before anything goes
  in (C3, Sherlock2's u136 condition). It is not built in this repository. Until it exists, D4 is
  exercised by a synthetic fixture shaped exactly as D4 says. The fixture names its source as
  this note, and it marks itself as synthetic.

## One-way doors

- **The new manifest id `capsulectl.deal-view-composed/v0`, and the module's pins.** These are
  frozen at release. This is engineering's call: the id carries no product meaning, and the
  release is pre-approved for its named consumers.
- **What a counterparty can see or what is disclosed:** unchanged. The page reads only what the
  composition already carries. The keep copy stays out of the bundle.
- **Copy:**
  - the case banners;
  - the refusal, absence and no-record lines;
  - stamp 3's new words;
  - "not stated" for their words.

  These are wording, for Sherlock2 under the delegation.
- **C4 lines ("as agreed", "paid" from a Mesh record):** not drawn. If they are wanted later,
  they go to Steven.

## Tests

- **Golden pages** from synthetic compositions:
  - stamped by both, for `x-deal-v0` and for Mesh;
  - their member failing each of V1-V4 in turn;
  - refusal, absence, and no member;
  - a mismatched keep copy;
  - a block whose declared join disagrees with the derived one.
- **Two rounds:** a test as D6 describes.
- **The single-book page** stays as #83 drew it.
- **Every page renders at 390 px** and is compared with frames 6 and 7.
