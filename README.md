# evidencebook-deal

`capsulectl-deal`, the `capsulectl` plugin that checks an agent's deal before
the agent pays, books, signs or shares on a user's behalf, and the `deal`
skill that drives it. It is the deal application on evidence books: each step
is sealed as a Capsule into the deal's own log on the profile's evidence-book
shelf ([evidencebook](https://github.com/action-state-group/evidencebook)),
and a deal's copies verify with `capsulectl verify` alone.

It moved here from [capsule-cli](https://github.com/action-state-group/capsule-cli)
(capsule-cli#326), where the plugin and the skill lived until then; their
history is capsule-cli's. `capsulectl` itself, and the `cli-plugin/v1`
contract this plugin implements, stay in capsule-cli
([docs/PLUGINS.md](https://github.com/action-state-group/capsule-cli/blob/main/docs/PLUGINS.md)).

## What is here

| Path | What it holds |
|---|---|
| `cmd/capsulectl-deal` | the plugin binary: the `capsulectl` command tree plus `capsulectl deal ...` |
| `internal/deal` | the deal commands, the deal store and its records, and the deal view (`assets/deal-view.*`), the plugin's presentation module |
| `skills/deal/` | the `deal` skill: `SKILL.md`, its README, demos and scripts. The x-deal-v0 record profile it seals is EvidenceBook's [`dealprofile/x-deal-v0/`](https://github.com/action-state-group/evidencebook/tree/v0.10.0/dealprofile/x-deal-v0) |
| `third_party/unicode/` | Unicode's TR39 `confusables.txt`, from which `scripts/genconfusables` builds `internal/deal/confusables_table.go` (a security control: the share check matches a sensitive value written with lookalike letters) |
| `testdata/presentation/` | the deal and report pages' presentation fixtures and goldens |
| `scripts/` | `test.sh`, the page and portability checks, the interop round trip, and the confusables tools |

## Build and test

The plugin embeds the `capsulectl` core it builds against, so it pairs with
one capsule-cli release: `go.mod` requires capsule-cli at a version, and the
plugin's handshake reports it as `core_version`. A release takes that same
version (RELEASING.md).

```sh
go build -o capsulectl github.com/action-state-group/capsule-cli/cmd/capsulectl
go build -o capsulectl-deal ./cmd/capsulectl-deal
./capsulectl plugin install ./capsulectl-deal
bash scripts/test.sh
```

The deal view (`capsulectl.deal-view/v0`) is the plugin's presentation
module: the deal section of a deal receipt's page. The plugin's own pages
(`deal report --html`) carry it from the binary. To let `capsulectl
presentation list` and `presentation build` use it too, put its files beside
the installed launcher, in a `capsulectl-deal.d/` directory: a release ships
them as `deal-view-VERSION.tar.gz`, unpacked into the plugin directory
(`tar -xzf deal-view-VERSION.tar.gz -C ~/.local/lib/capsulectl/plugins`).
Without them, `capsulectl plugin ls` reports the plugin's presentations
refused (`missing_file`) and its commands run as before.

A plain build is a development build of both: the plugin's `core_version`
reads `0.1.0-dev`, which a development `capsulectl` does not check, so build
the core from this module, as above. A released `capsulectl` takes only a
released plugin built for it.

`scripts/test.sh` runs the race pass, then the plain pass. CI also runs the
browser checks (`scripts/check-deal-pages.sh`, `scripts/check-portability.sh`
and the presentation tests in pinned Chrome and fonts) and the Go and Python
interop round trip at the versions this module resolves.

## License

Apache-2.0. See [LICENSE](LICENSE).
