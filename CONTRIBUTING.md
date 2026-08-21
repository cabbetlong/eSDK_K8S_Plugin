# Contributing

## Testing

Run the Phase 1 gate before opening a PR or committing:

```sh
make test-integration
```

For the full picture (still WIP under Go 1.26, see `test/README.md`):

```sh
make test
```

### Guidelines

1. **No new gomonkey.** The project is removing gomonkey from the integration
   layer (then repo-wide in batches). Use `gomock` or hand-written fakes, or
   introduce a test seam (replaceable var / constructor injection) instead.
2. Keep the fast integration layer green: fix both product code and tests in
   the same change, never silently delete a failing test.
3. When touching a test file that still uses gomonkey, migrate it to the new
   seam as part of that change (table-driven + shared fixture when sensible).
4. New E2E/component-integration tests go under `test/e2e` (hermetic: fake k8s
   client + fake storage + real gRPC; run with `make test-e2e`).

See `test/README.md` and `docs/adr/0001-test-strategy.md` for details.
