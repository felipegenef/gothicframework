## v2.17.3

### Bug Fixes

- **migrate-v2:** Fixed a critical error where `migrate-v2` seeded `go.mod` with the hardcoded version `v2.0.0` (which does not exist on the registry), causing `go mod tidy` to fail. The command now seeds the `github.com/felipegenef/gothicframework/v2` requirement with the current CLI version, guaranteeing the seeded version is resolvable.

### Infrastructure

- **CI:** Added `.github/workflows/ci.yml` — all Go tests must pass before a PR can be merged into `main`.
- **Branch cleanup:** Enabled "Automatically delete head branches" in repository settings — merged branches are deleted automatically after a PR is closed.
