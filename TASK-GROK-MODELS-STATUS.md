# Grok model availability and selective upstream release journal

Updated 2026-09-09T09:55Z. This is the authoritative journal for the selective release investigation.

## Release state

- Production baseline before this release: `https://grok.cheapaiapi.org`, `ghcr.io/ohld/grok2api:sha-6d5f9a093bb0d00bb40802f443c49b99c9a83fb`.
- Reviewed selective branch: `codex/selective-upstream-grok-models`, merged by [PR #3](https://github.com/ohld/grok2api/pull/3) as merge commit `6e45a998602dedd0e8950ab3c6721372c99dfd48` at 2026-09-09T09:48:28Z.
- The main push is publishing through [GHCR Image run 34336777924](https://github.com/ohld/grok2api/actions/runs/34336777924). The post-deploy serving digest and rollback record will be appended after Coolify converges.
- Rollback target is the previous immutable image `ghcr.io/ohld/grok2api:sha-6d5f9a093bb0d00bb40802f443c49b99c9a83fb`; no database migration is part of this release.

## Included changes

- [df4dde39](https://github.com/chenyme/grok2api/commit/df4dde39ca8dcaf66efc0a197a7ffdc4df1229bd) and [6db9f67](https://github.com/chenyme/grok2api/commit/6db9f67fbe55d3986c566b436dff0b555775988): Console request normalization drops `tool_choice` when valid tools are absent, with regression coverage.
- [e5285ebe](https://github.com/chenyme/grok2api/commit/e5285ebeb6fadaa13231e19a88bf4be2a43ae43a), plus owned fixes in `6f74125b`, `6aea2ad1`, and `b38e751e`: Build function-schema union filtering normalizes each branch first, preserves nullable object branches, resolves local references for classification, and filters scalar references consistently.
- The final diff remains limited to four provider normalization/test files. It does not change model routes, quotas, account state, image capacity, admission, persistence, or migrations.

## Deliberately omitted upstream work

- [PR #1041](https://github.com/chenyme/grok2api/pull/1041), [ca392e68](https://github.com/chenyme/grok2api/commit/ca392e6890ac5018d82e97419b533db8c3e5aa51), and [7d1b4246](https://github.com/chenyme/grok2api/commit/7d1b424626a8a2c6969d66598fc4b4ebe8785b7b) were omitted. The reasoning replay patch conflicts with six files and replaces replay lineage absent from the deployed baseline; its follow-up overflow fix targets that new helper. Partial import would risk native Responses compatibility and process-local replay semantics.
- The current upstream snapshot is `main` at [`8913b53f`](https://github.com/chenyme/grok2api/commit/8913b53fe92307a6f111b2885ab298a43c74a9ba), commit time 2026-09-09T15:43:06+08:00, merged as [PR #1043](https://github.com/chenyme/grok2api/pull/1043). The latest upstream release tag is [`v3.1.5`](https://github.com/chenyme/grok2api/releases/tag/v3.1.5), object `88ef5bd948001924a8f71c9e8ac8b8dfacace5b0`.

## Live model evidence before release

- Console `grok-4.5` returned HTTP 200 for bounded authenticated probes.
- Console `grok-4.20` routes had zero remaining quota and returned HTTP 429 `upstream_quota_exhausted`.
- Build `grok-4.6` was a nominal route but the existing client key was not scoped to Build; a bounded request returned HTTP 503 `client_key_account_scope_unavailable`.
- Build discovery observed `grok-4.5`; its only account was active/enabled but expired and repeatedly failing OAuth transport refresh. The Build `grok-4.6` capability is real when returned by valid Build model discovery; no absolute quality ranking is claimed.
- `/healthz` returned 200. `/readyz` was degraded because Statsig remained degraded and no enabled Build route was available; Console remained ready.
- CheapAIAPI `https://cheapaiapi.org`: `/health` and `/ready` returned 200; recent generation diagnostics were healthy with zero operational failures, provider 5xx, and zero-attempt failures. Overall status was degraded only for `fixed_catalog_repair_worker: release_mismatch_heartbeat:1`.

## Bounded account and OAuth checks

- Account inventory at probe time: 447 active Console accounts (442 enabled), 450 active Web accounts (395 enabled), and one active/enabled Build account. The existing client key was scoped to Web+Console with allowed model IDs `[7,11,13,22]` and max concurrency 8.
- Three simultaneous low-volume Console `grok-4.5` requests through the existing key all returned HTTP 200, echoed their unique request IDs, and matched exact admin audit rows. The three rows were timestamped 09:40:02.063Z, 09:40:02.233Z, and 09:40:06.615Z and showed three distinct anonymized account hashes during the audit check. No account identifiers, request identifiers, prompts, credentials, or raw payloads are retained here.
- One authorized refresh was attempted for the sole existing Build account and returned HTTP 502; the account stayed active/enabled with `oauth_transport_error`. Two existing active Web accounts were selected for one bounded Web-to-Build conversion attempt; the operation completed with zero creations and two failures. No registration, purchase, bulk conversion, or retry loop was used.

## Validation

- `go test ./internal/infra/provider/cli ./internal/infra/provider/console`: PASS.
- `go vet ./internal/infra/provider/cli ./internal/infra/provider/console`: PASS.
- `go test ./...`: PASS.
- Independent architecture and implementation reviews approved the final `b38e751e` branch after the nullable-object and scalar-reference blocker was fixed.

## Relevant upstream evidence

- [Issue #943](https://github.com/chenyme/grok2api/issues/943) and [comment](https://github.com/chenyme/grok2api/issues/943#issuecomment-5302816744): Console `grok-4.6` was not available in the upstream console path at review time.
- [Issue #905](https://github.com/chenyme/grok2api/issues/905), [PR #906](https://github.com/chenyme/grok2api/pull/906), and its [fix summary](https://github.com/chenyme/grok2api/issues/905#issuecomment-5276086387): sparse Build model discovery needed to preserve `grok-4.5` compatibility.
- [Issue #902](https://github.com/chenyme/grok2api/issues/902) and [stability comment](https://github.com/chenyme/grok2api/issues/902#issuecomment-5310827079): upstream reports described recurring 4.6 quota failures and steadier 4.5 behavior.
- [Issue #916](https://github.com/chenyme/grok2api/issues/916) and [egress-risk comment](https://github.com/chenyme/grok2api/issues/916#issuecomment-5282191035): upstream discussion identified shared-IP and batch-account risk for Build quality.
