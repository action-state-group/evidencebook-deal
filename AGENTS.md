# Contributor guidance

This repository holds the `capsulectl-deal` plugin and the `deal` skill
(capsule-cli#326).

- The plugin builds against capsule-cli's host package
  (`github.com/action-state-group/capsule-cli/cli`), required at a capsule-cli
  version, never replaced. Changes the core must make (a hook, a check, a
  wording key) land in capsule-cli first.
- Reuse evidencebook and capsule-cli; do not reimplement AAC digest or
  signature rules, or a verifier: `capsulectl verify` is the verifier.
- Keep the deal record's wire shape in `skills/deal/profile/` (PROFILE.md and
  its schemas); once the Agent Deal Records spec exists it is the source and
  PROFILE.md cites it.
- Use testify assert/require in tests.
- Before a commit, run `gofmt`, `go mod tidy`, `go vet ./...` and
  `bash scripts/test.sh`. Use DCO sign-off and conventional commit messages.
  Never commit secrets or local plans.
