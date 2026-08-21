# ADR-0001: Test strategy for integration and E2E testing

- Status: Accepted
- Date: 2025
- Areas: testing, CI, test tooling

## Context

`test/integration/` contained in-process CSI tests heavily reliant on
`gomonkey`. After `go.mod` moved to Go 1.26.1 (v4.12.0), compiler inlining
broke many gomonkey patches: 22+ packages fail under `go test ./...`. There is
no automated test gate in CI, and no true E2E coverage.

## Decision

Adopt a two-tier test strategy and progressively remove gomonkey:

1. **Fast integration layer** (`test/integration/`) — in-process, mock-based,
   fast. Keep it green and make it the Phase 1 gate (`make test-integration`).
2. **Hermetic integration layer / E2E** (`test/e2e/`) — real CSI gRPC server +
   the real backend sync job (`handler.BackendRegister.FetchAndRegisterAllBackend`)
   + a real HTTP storage client against an in-memory fake Huawei array
   (`test/fake/storage`). The Kubernetes side uses the standard client-go fake
   clientset, so no test cluster or external binaries are required. First
   milestone **implemented**: OceanStor SAN CreateVolume -> DeleteVolume through
   the real CSI gRPC server. The storage-backend controller Claim→Content
   reconcile loop is a follow-up (Phase 2.1); until then the tests create the
   StorageBackendContent directly. Envtest/kind are intentionally deferred to
   Phase 2.1, when real finalizer/status/GC semantics matter and fake clients
   are no longer faithful.

3. **Remove gomonkey progressively**, integration layer first, repo-wide in
   batches. Injection strategy: replaceable package vars first, constructor
   injection later. Migration order:
   1. `app.GetGlobalConfig` / `K8sUtils`
   2. storage client factories (e.g. `client.NewIRestClient`)
   3. `backend.GetStorageBackendInfo`, `host.GetNodeHostInfosFromSecret`
   4. volume seams (`volume.DTree/NAS`, etc.)
4. Quality gates: `go vet` + `go test ./... -count=1` + coverage report
   (coverage is recorded, not enforced as a threshold yet). CI automation is
   deferred; the local Makefile gate is the source of truth for now.

## Temporary workarounds

`//go:noinline` was added to `client.NewIRestClient`,
`(*DTree).AutoManageAuthClient` and `(*NAS).AutoManageAuthClient` so gomonkey
can patch them under Go 1.26. **Remove these directives when the corresponding
seam is migrated** (steps 2 and 4 above).

## Consequences

- `test/integration` is green and gateable immediately.
- Full-repo `go test ./...` remains red until the gomonkey migration catches
  up; the failing package list is tracked (see `test/README.md`).
- New code must not introduce gomonkey.
