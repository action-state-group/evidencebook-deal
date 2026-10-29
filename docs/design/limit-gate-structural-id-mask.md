# Design note: the spending-limit gate masks ids only inside real JSON

Issue: https://github.com/action-state-group/evidencebook-deal/issues/62.
Follows https://github.com/action-state-group/evidencebook-deal/pull/60 (5a19f35, fix for #26).
Status: draft for review (Codex, Kimi or its Claude stand-in, Sherlock2). No code until it converges.

## Problem

`dealCeilingGate` (`internal/deal/deal_share_ceiling.go`) refuses a shared copy that states the
user's spending limit (counterparty copies) or floor (every shared copy) as money. #60 stopped
random refusals (#26) by masking the leading digits of a JSON string value that is entirely one
random base64url id (`maskOpaqueIDs`), so `"cose":"8krSil…"` is not read as 8 kr.

The mask finds ids with a pattern, `":\s*"([0-9A-Za-z_-]{16,})"` plus a mixed-case check, and it
runs on the folded text (`foldText`). A pattern on folded text cannot tell a JSON value from prose
that looks like one. Known false negative, accepted by Ethan on 2026-10-29 as a declared residual:
with a limit of 8.00, `My "limit": "8USD-Per-Month-Plan-2026".` loses its leading `8` to the mask
and the limit is not read.

## Where the gate's input comes from

`dealShareGate` is the only caller of `dealCeilingGate` outside tests
(`deal_share_ceiling.go:282`). It has five callers, and each passes one of two kinds of bytes:

| Caller | Bytes | Kind |
| --- | --- | --- |
| `deal_bundle.go:100` (`disclose`) | `json.Marshal(b)` | JSON document |
| `deal_history.go:316` (history copy) | `json.Marshal(b)` | JSON document |
| `deal_terms_seal.go:425` (terms Result bundle) | `json.Marshal(b)` | JSON document |
| `deal_evidence.go:742` (evidence-request copy) | `canonical.JCS(...)` | JSON document |
| `deal.go:2562` (`report --html`) | the emitted page | HTML page |

The gate does not read these bytes directly. `dealShareGate` passes `dealGateText(data)`
(`deal_share_gate.go:94`), which:

- for a copy that does not start with an HTML doctype, returns the whole copy less the two vendored
  scripts (`dealWithoutVendored`); for the four JSON callers that is the JSON document;
- for a page, returns the markup outside every `<script>` and `<style>`, plus the JSON value of each
  of the two data scripts (`window.__BUNDLE__ =` and `globalThis.capsulectlDealPage =
  Object.freeze(`), each cut out with `json.Decoder` (`deal_share_gate.go:113-125`). A data script
  that does not decode is kept whole as text.

So the real JSON regions are already delimited: the whole copy for the four bundle callers, and the
two data-script values of a page. Everything else is prose: page markup (the title and the
emitter's shell), a data script that does not decode, and any copy that is not valid JSON.

`dealCeilingGate` then folds the text (`foldText`: percent-escapes, HTML character references and
`\uXXXX` escapes decoded repeatedly, `\\` read as `\`) and only then masks. Folding is what lets
prose reach the mask looking like JSON:

- page markup carries quotes as `&#34;`, which folding turns into `"`;
- a JSON string whose text holds a literal `"` is written by `json.Marshal` as `\\u0022`, which
  folding turns into `"` (a JSON string's own `\"` is not decoded, which is why the literal repro
  inside a bundle field is already refused today).

## Design

Mask the raw bytes of each real JSON region, before folding, and never anything else.

1. `dealGateText` gains a hook for the JSON regions it already cuts out: `dealGateText(page,
   jsonValue func([]byte) []byte)`. It applies `jsonValue` to each data-script value it decodes, and
   to a non-page copy only when the whole copy (less the vendored scripts) is valid JSON
   (`json.Valid`). Markup, a data script that does not decode, and a non-JSON copy are returned as
   now. `dealPageGate` passes `nil` and reads exactly what it reads today.
2. `dealShareGate` passes the raw copy to `dealCeilingGate`, which calls `dealGateText(data,
   maskOpaqueIDs)` itself, then folds and checks as now. The mask call after folding
   (`deal_share_ceiling.go:243`) is removed; the money checks read the folded, masked text.
3. `maskOpaqueIDs` keeps its rule (a string value after a key's colon that is entirely 16 or more
   of `[0-9A-Za-z_-]` with an upper-case and a lower-case letter; only its leading digits become a
   letter). Run on a valid JSON document's raw bytes, that rule is a structural match, not a guess:
   in valid JSON every unescaped `"` delimits a string token and every quote inside a string is
   written `\"`, so `":\s*"…"` can only match a key's closing quote, its colon and a whole string
   value. A quote escaped inside a string is preceded by `\`, which the id class excludes, and
   after a string's closing quote valid JSON allows only whitespace, `,`, `:`, `]` or `}`, never an
   id character. Nothing else in the region is touched.

The money reader (`deal_money_text.go`), the bound-name checks and `removeHexTokens` do not change.
The hook is one parameter on the function that already owns "which parts of a copy are data";
no new type, parser or field list.

Considered and not chosen:

- **A `json.Decoder` token walk** that masks by token position. Equivalent on valid JSON (see 3)
  but more code; worth it only if a reviewer finds a case the regex-on-valid-JSON argument misses.
- **Masking by field name** (`checkpoint.cose`, the cadence chain's `checkpoint.cose`,
  `payloads.<digest>`; from `evidencebook` `substrate_cll.go:332`, `cadence_chain.go:79`,
  `bundle.go:300`). It would also close the whole-value case below, but the base64url fields are
  an open set across evidencebook and the spec, so a field the list misses brings back the random
  refusals of #26, and keeping the list right needs a guard test over every copy shape. Not worth
  it while the case it adds is not reachable (below).

## Prose regions

Prose is never masked: page markup, the text of an undecodable data script, a copy that is not
valid JSON, and the text inside any JSON string (a quote in it is `\"` in the raw bytes, so the
mask cannot see it as structure). The bound-name checks keep reading the whole text, keys included.

## Residuals

Closed (missed on main, refused with this design; prototype results below):

- `My "limit": "8USD-Per-Month-Plan-2026".` as a non-JSON copy (the #62 repro).
- The same prose in page markup, with literal quotes or as `&#34;`.
- The same prose inside a JSON string written with literal `"` escapes, which folding decodes
  into quotes.

Left open, declared:

- **A JSON string value that is itself entirely an id-shaped slug starting with the bound**, such
  as `{"note":"8USD-Per-Month-Plan-2026"}` with a limit of 8.00. This design masks it as #60 does
  (missed on main too). Only masking by field would close it. It needs user-written text to be a
  whole field value in a shared copy, 16 or more characters of `[0-9A-Za-z_-]` in both cases,
  starting with the bound's exact digits. I found no path that puts user text there: a `change`
  record whose `terms.item` was `8USD-Per-Month-Plan-2026` was refused by the page gate as a
  private value, and `8 USD per month plan` was withheld from the counterparty copy (probe on
  d60c732; one field, not an exhaustive search).
- An id written with escapes (`8krSil…`) or non-ASCII digits is not masked and may be read as
  money: a false positive, the safe direction. Producers write ids in plain ASCII.

## Reachability and whether it is worth doing

None of the closed cases is reachable through today's five callers without user text: the four
bundle callers write valid JSON in which prose quotes are `\"`, and the counterparty and adjudicator
pages have fixed titles (`dealPageTitle`, `deal_report.go:489`) inside the emitter's shell. The one
reachable path is user text holding a literal `"`, inside a field that reaches a shared copy.

Recommendation: do it. The change is small (a hook on `dealGateText`, the mask moved before
folding, about 30 lines plus tests), it removes the gate's dependence on its callers writing prose
quotes as `\"`, and it turns the #60 residual comment into a structural guarantee with one narrow,
declared case left. If reviewers judge the benefit too small for a money-gate change, the
alternative is to close #62 with the residual as declared and keep #60's comment.

## Test vectors

Limit is in minor units, USD. "main" is `dealCeilingGate(dealGateText(x))` on d60c732; "design" is
a prototype of this note (`protoGateText` plus the gate without the post-fold mask), run as a
scratch test on d60c732. R = refused, P = passes.

Random ids (must pass), from `TestARandomIdIsNotTheLimit` and #60's rounds:

| Limit | Input | main | design |
| --- | --- | --- | --- |
| 800 | `{"cose":"8krSiljccsfh-Gc4lpkxi8m4j8wl"}` | P | P |
| 800 | `{"cose":"8krSiljccsfhGc4lpkxi8w"}` (R3: 22 characters) | P | P |
| 800 | `{"cose":"8krSiljccsfhGcAlpkxiAw"}` (R4: one digit group) | P | P |
| 800 | `{"cose":"8kR2HyiCRXnmH-sExilKR7VFhmKmkFs-A8P1Z2q_z_R"}` | P | P |
| 800 | `{"cose":"8gbp-Qlhh6xzsywqlv3kuyf8b1xtzkQ"}` | P | P |
| 800 | `{"cose":"8usd_K3j2h1xxxxxxxxxxxxxxxxxx"}`, `8eur-Ab3…`, `8chf-Xz9…` | P | P |
| 800 | a page whose bundle is `{"checkpoint":{"cose":"8krSiljccsfh-Gc4lpkxi8m4j8wl"}}` | P | P |

Bound stated (must refuse), from `TestTheLimitInOrBesideALongRunIsStillRefused` and the rounds:

| Limit | Input | main | design |
| --- | --- | --- | --- |
| 800 | `my limit is $8.00`, `8kr`, `limit 8usd-ish`, `my-limit-is-$8` | R | R |
| 800 | `limit 8.00 Zq3-Tr9xYb_Lm4NpWk7VcHd2Fg5Js8Aa`, and the id before it | R | R |
| 800 | `Lowest-I-Will-Take-Is-Exactly-8-Dollars-Thanks-ok limit is 8 usd` | R | R |
| 800 | `Max_Budget_Limit_FY2026Q4: 8` (R3) | R | R |
| 800 | `My-Personal-Spending-Limit-2026 is 8`, `My-Absolute-Spending-Limit-Is-8.50 and 8.00` (R3) | R | R |
| 800 | `8 usd_Settlement_Ref_A1b2c3d4`, `8USD-Per-Month` | R | R |
| 800 | `{"note":"8krSiljccsfh-Gc4lpkxi8m4j8wl and my limit is 8"}` | R | R |
| 800 | `{"note":"my limit is 8 USD"}`, `{"note":"8USD-Per-Month-Plan-2026-Ok limit"}` | R | R |
| 800 | `{"8krSiljccsfh-Gc4lpkxi8m4j8wl":"limit 8 usd"}` (id as a key) | R | R |
| 800 | `My limit is "8USD-Per-Month-Plan-2026".` (R5) | R | R |
| 800 | `{"note":"My limit is \"8USD-Per-Month-Plan-2026\"."}` | R | R |
| 800 | `["8USD-Per-Month-Plan-2026"]` | R | R |
| 800 | `{"max_total_minor":800}`, `{"x_max_total_minor_Zq3-…":1}` | R | R |
| 850 | `$8.50USD-Per-Month-Plan-2026` (R4) | R | R |
| 80000 | `USD700-USD800`, `800USD-900USD`, `800usd-ish` (R1, R2) | R | R |
| 800000 | `$8k-per-month`, `{"pay":"8k-4Qwxyz"}`, `USD7k-USD8k`, `USD8k-per-month`, `my limit is 8k-per-month` (R1, R2) | R | R |
| 80000 | `my limit is 800USD- final` (R2) | R | R |
| 800 | `$8kr`, `8 kr` (R2) | R | R |
| 80000 | `Max_Budget_Limit_FY2026Q4: 800`, `800 USD_Settlement_Ref_A1b2c3` (R3) | R | R |
| 800 | `{"note":"My \"limit\": \"8USD-Per-Month-Plan-2026\"."}` | R | R |
| 800 | `{"note":"…"` with a trailing `, }` (invalid JSON: read whole, no mask) | R | R |

The #62 residual:

| Limit | Input | main | design |
| --- | --- | --- | --- |
| 800 | `My "limit": "8USD-Per-Month-Plan-2026".` (R6, the #62 repro) | P | R |
| 800 | `{"note":"My \\u0022limit\\u0022: \\u00228USD-Per-Month-Plan-2026\\u0022."}` | P | R |
| 800 | a page whose title is `My &#34;limit&#34;: &#34;8USD-Per-Month-Plan-2026&#34;.` | P | R |
| 800 | a page whose title is `My "limit": "8USD-Per-Month-Plan-2026".` | P | R |
| 800 | `{"note":"8USD-Per-Month-Plan-2026"}` (declared residual) | P | P |
| 800 | a page whose bundle is `{"note":"8USD-Per-Month-Plan-2026"}` (declared residual) | P | P |

The code PR adds every row above to the gate's tests, the declared-residual rows asserting today's
outcome with a comment, so a later change to them is deliberate.

Real-copy fuzz, kept as a test: the counterparty bundle and page from the
`openShortMerchantDeal` fixture, `checkpoint.cose` replaced by a random 400-character base64url
value starting with `8` (half of them `8kr`), fixed seed, against a limit of 8.00. Acceptance: 0
refusals. #60 recorded 21 of 20,000 on its main and 0 on its head with its own scratch harness;
this note's prototype numbers are under "Prototype".

## Acceptance

- Every vector above has the "design" outcome.
- The #60 tests (`TestARandomIdIsNotTheLimit`, `TestRandomBase64urlIdsNeverStateTheLimit`,
  `TestTheLimitInOrBesideALongRunIsStillRefused`, `TestDealCeilingGate`) pass unchanged.
- The real-copy fuzz test shows 0 refusals.
- `dealPageGate` reads the same bytes as before (its tests pass unchanged).
- The residual comment on `maskOpaqueIDs` names only the declared whole-value case.

## Prototype

A scratch prototype on d60c732 (not committed): `dealGateText` with the hook, and the gate body
with the mask moved before folding. Besides the vector outcomes above, the real-copy fuzz
(fixed seed 62, limit 8.00):

| Copy | Rounds | d60c732 | design | no mask at all |
| --- | --- | --- | --- | --- |
| counterparty bundle | 20,000 | 0 | 0 | 9,953 |
| counterparty page | 2,000 | 0 | 0 | 1,028 |

The "no mask" column shows the harness exercises the #26 failure (half the rounds start `8kr`).
