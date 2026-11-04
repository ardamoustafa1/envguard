## Summary

What changes and why. Link the issue if there is one.

## Checklist

- [ ] `make test` and `make lint` pass.
- [ ] New behavior has tests (table-driven). Goldens were updated deliberately and the diff reviewed.
- [ ] `CLAUDE.md` is updated if architecture, rules, exit codes or conventions changed.
- [ ] No secret value can reach any output (terminal, JSON, SARIF, GitHub annotations, errors, verbose logs). Value text flows through `internal/mask`. `TestLeakCanary` still passes.
- [ ] No new network-capable imports in non-test code (`TestOffline`).
- [ ] Output remains deterministic.
- [ ] Fixtures contain no real-looking secrets (use `{{secret:KIND}}` or `testutil.FakeSecret`).
- [ ] Docs are updated. See [CONTRIBUTING.md](../CONTRIBUTING.md) for the guides.
