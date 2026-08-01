## Summary

<!-- What changed and why? -->

## Verification

- [ ] `make check`
- [ ] `go test -race ./...` when Go behavior changed
- [ ] `pnpm --dir web run test:e2e` when user workflows changed
- [ ] No credentials, tokens, project IDs, queries, or real log data were added
- [ ] Generated API artifacts were updated with `make api` when the contract changed
