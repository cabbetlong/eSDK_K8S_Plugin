# Testing Guide

This project keeps two clearly separated test layers:

| Layer | Directory | Scope | Requires |
|-------|-----------|-------|----------|
| Fast integration | `test/integration/` | In-process CSI RPC + mock storage clients; fast, no external deps | `go test` |
| E2E (opt-in) | `test/e2e/` | Real kube-apiserver/etcd (envtest) + fake in-process storage server + real gRPC socket | `make test-e2e` (auto-downloads envtest) |

## Commands

```sh
make test-integration    # Phase 1 gate: fast integration layer
make test-e2e            # envtest-based E2E: CreateVolume -> DeleteVolume (OceanStor SAN)
make setup-envtest       # download pinned kube-apiserver/etcd binaries (KUBEBUILDER_ASSETS)
make coverage            # coverage profile + HTML report (integration layer)
make test                # WIP: vet + full repo (not fully green yet, see below)
```

The E2E milestone currently covers **CRD + real sync job + real gRPC
CreateVolume/DeleteVolume against a fake OceanStor SAN** (Content is created
directly; the Claim→Content storage-backend-controller reconcile is a planned
follow-up).

Coverage baseline (recorded, not enforced):

```sh
make coverage
# integration layer over csi/storage/utils/pkg: ~10.6% (2025-08, coverage.out/coverage.html)
```

All `make` test targets pass `-count=1` to bypass the Go test cache for reproducible results.

## Known status (Go 1.26)

`go.mod` is on Go 1.26.1 (bumped in v4.12.0). Several gomonkey-based tests are
broken by compiler inlining — 22+ packages currently FAIL under `go test ./...`.
This is a known, tracked issue:

- The fast integration layer (`test/integration/...`) is **green**.
- The remaining packages are being migrated away from gomonkey in batches
  (see `docs/adr/0001-test-strategy.md` for the seam-by-seam plan).
- Temporary `//go:noinline` workarounds exist on `client.NewIRestClient`,
  `DTree.AutoManageAuthClient` and `NAS.AutoManageAuthClient`. They must be
  removed when those seams are migrated to dependency injection.

Therefore `make test` (full repo) is intentionally **not** the Phase 1 gate;
`make test-integration` is.

## Writing tests

- Keep new tests deterministic: no real network, no real storage, no sleep.
- Prefer `gomock`/hand-written fakes over gomonkey; **do not add new gomonkey
  usage** (goal: zero gomonkey in the integration layer).
- New test cases may use `t.Parallel()` when they touch no shared global state
  (the backend cache and global config are shared — be careful).
- Follow existing arrange/action/assert structure; migrate to table-driven
  tests when you touch a file.
- Shared fixtures live under `test/utils` (and later `test/fake` for the fake
  storage server).
