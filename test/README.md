# Testing Guide

This project keeps two clearly separated test layers:

| Layer | Directory | Scope | Requires |
|-------|-----------|-------|----------|
| Fast integration | `test/integration/` | In-process CSI RPC + mock storage clients; fast, no external deps | `go test` |
| Hermetic integration | `test/e2e/` | Real CSI gRPC server + real sync job + real HTTP storage client against an in-memory fake Huawei array; Kubernetes side uses client-go fake clientset | `go test ./test/e2e/...` (no external binaries) |

## Commands

```sh
make test-integration    # Phase 1 gate: fast integration layer
make test-e2e            # hermetic integration: CreateVolume -> DeleteVolume (OceanStor SAN)
make coverage            # coverage profile + HTML report (integration layer)
make test                # WIP: vet + full repo (not fully green yet, see below)
```

The `test/e2e` milestone covers:

- **Controller side**: real sync job + real gRPC CreateVolume/DeleteVolume
  against a fake OceanStor SAN; Kubernetes side backed by the standard fake
  clientset (no envtest binary needed). Content is created directly; the
  Claim→Content storage-backend-controller reconcile is a planned follow-up
  that will need envtest/kind.
- **Node side (hermetic)**: NodeGetCapabilities / NodeGetInfo /
  NodeGetVolumeStats and an NFS Stage → Publish → Unpublish → Unstage chain
  through the real gRPC node server. OS operations are injected via the
  replaceable seams `utils.GetHostNameFunc`, `connector/utils.MountToDirFunc`
  and `connector/utils.UnmountFunc`, so the tests need no privileges or real
  mounts/arrays.

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
