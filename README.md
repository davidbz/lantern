# golang-template

An opinionated template for Go applications: Go 1.27, strict linting, reproducible tooling, a devcontainer,
supply-chain-hardened CI, and signed releases.

## What's included

| Area | Tooling |
| --- | --- |
| Toolchain | Go 1.27; `go.mod` is the single source of the Go version for CI, Docker and lint |
| Project tools | mockery v3, govulncheck, go-licenses as `tool` directives in `go.mod` (`go tool <name>`) |
| Lint & format | golangci-lint v2 (~80 linters, gofumpt/goimports/golines), see `.golangci.yml` |
| Tests | testify + mockery, `-race -shuffle=on`, coverage |
| Dev environment | Devcontainer (Debian trixie) with pinned gopls, dlv, golangci-lint, goreleaser; Docker via host socket |
| CI | Build/test, tidy & mocks drift checks, golangci-lint, zizmor, TruffleHog, govulncheck (also weekly), license check |
| Container | Multi-stage `Dockerfile` to `distroless/static:nonroot` |
| Release | GoReleaser on `v*` tags: multi-platform archives, SBOMs, ko-built images on GHCR, cosign keyless signing, build provenance attestations |
| Supply chain | Actions pinned by commit SHA, Dependabot (actions, gomod, docker, devcontainers) with grouping and a 7-day cooldown |
| AI agents | `AGENTS.md` (`CLAUDE.md`/`GEMINI.md` symlink to it) and Copilot instructions in `.github/instructions/` |

## Using this template

1. Click **Use this template** on GitHub, then open the repo in the devcontainer.
2. Replace the module path `github.com/davidbz/golang-template` in:
   - `go.mod`
   - `.golangci.yml` (`formatters.settings.goimports.local-prefixes`)
   - `.vscode/settings.json` (`gopls.formatting.local`)
3. Set `project_name` in `.goreleaser.yaml` and update `LICENSE`.
4. Add your entry point at `cmd/app/main.go` (or `cmd/<name>/` and pass `CMD=./cmd/<name>` to make).
   Declare `var version = "dev"` in `main` to receive the version stamped at build time.
5. Register packages with interfaces to mock under `packages:` in `.mockery.yaml`.
6. Run `make ci`.

Until there is Go code, the build, test and vulnerability targets skip with a message instead of failing.

## Make targets

Run `make` (or `make help`) for the full list.

| Target | Description |
| --- | --- |
| `build` / `run` | Build or run `$(CMD)` (default `./cmd/app`) |
| `test`, `test-coverage`, `test-coverage-html` | Tests with race detector and shuffle; coverage reports |
| `mocks`, `mocks-regen`, `mocks-check` | Generate mocks; fail if committed mocks are stale |
| `fmt`, `lint`, `lint-fix` | golangci-lint formatters and linters |
| `vuln`, `licenses` | govulncheck; fail on forbidden/restricted dependency licenses |
| `tidy`, `tidy-check` | `go mod tidy`; fail if not tidy |
| `docker` | Build the container image |
| `release-snapshot` | Local GoReleaser dry run into `dist/` |
| `ci` | Everything CI checks |

## Releasing

Push a semver tag:

```bash
git tag v0.1.0 && git push origin v0.1.0
```

`.github/workflows/release.yml` runs GoReleaser, which publishes archives, checksums and SBOMs to a GitHub release
and images to `ghcr.io/<owner>/<project_name>`, signs them with cosign (keyless), and attests build provenance.
Verify an image with:

```bash
cosign verify ghcr.io/<owner>/<project_name>:<version> \
  --certificate-identity-regexp 'https://github.com/<owner>/<repo>/' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
gh attestation verify <archive> --repo <owner>/<repo>
```

## Updating versions

Dependabot updates Go modules (including tools), actions, and base images. Two things need manual bumps:

- **Go**: change the `go` line in `go.mod` and `run.go` in `.golangci.yml`; the Docker/devcontainer base image tags
  follow via Dependabot.
- **golangci-lint**: keep `.devcontainer/Dockerfile` (`GOLANGCI_LINT_VERSION`), `.github/workflows/lint.yml`
  (`version`) and the comment in `.golangci.yml` on the same release.

## License

[MIT](LICENSE)
