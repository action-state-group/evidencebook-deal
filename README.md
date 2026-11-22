# evidencebook-deal has moved

`capsulectl-deal`, the `capsulectl` plugin for deals, and the `deal` skill now live in
[action-state-group/evidencebook](https://github.com/action-state-group/evidencebook),
under [`capsulectl/`](https://github.com/action-state-group/evidencebook/tree/main/capsulectl),
with their commit history (action-state-group/evidencebook#610,
action-state-group/evidencebook#660). Open issues were transferred there. This repository
holds nothing else and is archived.

| It was | It is now |
|---|---|
| `cmd/capsulectl-deal`, `internal/deal` | `capsulectl/cmd/capsulectl-deal`, `capsulectl/internal/deal` |
| `skills/deal` | `capsulectl/skills/deal` |
| `examples/` | `capsulectl/examples/` |
| `CHANGELOG.md`, `RELEASING.md` | `capsulectl/CHANGELOG-deal.md`, `capsulectl/RELEASING.md` |
| releases `v0.1.0-rcN` | releases `capsulectl/v0.1.0-rcN` of evidencebook, which carry both `capsulectl-book` and `capsulectl-deal` |

Releases up to `v0.1.0-rc45` stay here with their assets, and their links keep working. The next
release ships from evidencebook, and `capsulectl-deal` there is built and released with
`capsulectl-book` from one module.

The history of this repository before the move is also kept here: `git log` on the commit
before the one that removed the files.
