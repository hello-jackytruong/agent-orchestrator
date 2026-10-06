# CLIProxyAPI and AO account-manager responsibility analysis

Research snapshot: 2026-10-06.

This note records the comparison between AO's provider-account work on
`codex/cliproxy-account-manager` and the locally embedded CLIProxyAPI v8.0.8
module. It includes the committed branch and the current uncommitted trimming
worktree. The note belongs only to this branch.

## Conclusion

AO is not reimplementing all of CLIProxyAPI. CLIProxyAPI is the provider
credential, token, protocol, and inference engine. AO is the account-policy,
session-routing, durability, recovery, and product layer.

The committed backend and proxy-host diff adds approximately 19,050 lines:

- 13,506 lines are Go tests.
- 5,544 lines are production Go code.
- The frontend and generated API artifacts add further lines.

The production code is therefore much smaller than the headline PR diff. The
remaining production code is also concentrated around semantics that the
CLIProxyAPI management API does not provide.

## Responsibility split

| Area | CLIProxyAPI provides | AO still provides | Assessment |
| --- | --- | --- | --- |
| Credential storage and refresh | Credential files, token refresh, auth inventory, deletion | AO account IDs, labels, signed-in state, and primary account | Mostly delegated already |
| Browser OAuth | Auth URL, callback, status, and cancellation | Loopback callback relay and association with an AO account | AO wrapper remains necessary for isolation and correlation |
| Credential import | Native `/v8/management/credentials` upload | Input validation and AO account association | The current worktree delegates this correctly |
| API keys | Native v0 provider-key configuration endpoints | AO account identity and deletion policy | Mostly delegated already |
| Normal inference | Provider protocol translation, retries, token refresh, and model routing | Session-ticket validation and exact account selection | AO boundary remains necessary |
| Per-session routing | Internal SDK support for a pinned auth ID | Session-to-account map, signed tickets, route revisions, and leases | Required by the current product |
| Durable recovery | Provider credential/config persistence | SQLite state, pending mutations, and crash recovery | Required by AO |
| Usage display | Generic authenticated calls and plugin quota APIs | Codex/Claude response parsing, normalization, caching, and UI DTOs | Partial delegation opportunity |
| Quota auto-switch | Provider error detection and cooldown mechanisms | AO policy for changing only the primary and affected sessions | AO-specific behavior |

## Code that is genuinely reusable from CLIProxyAPI

The current branch already uses the native CLIProxyAPI surfaces for most
credential operations:

- `POST /v8/management/credentials` for imported credential files.
- `GET`, `PUT`, and `DELETE` on the v0 Codex/Claude API-key configuration
  endpoints.
- `GET /v8/management/oauth/auth-url` for browser login.
- `GET /v8/management/oauth/status` and `DELETE /v8/management/oauth/session`
  for browser-login polling and cancellation.
- `GET`, `POST`, and `DELETE /v8/management/credentials` for inventory and
  credential cleanup.

The AO adapter around these calls is not another credential implementation. It
validates input, attaches an AO login ID, waits for the helper inventory to
reflect the operation, and records the resulting credential against the right
AO account.

## Strongest redundancy: Codex device login

[`proxy-host/internal/host/account_inputs.go`](../../proxy-host/internal/host/account_inputs.go)
contains a custom Codex device flow that calls the OpenAI device endpoints,
polls for authorization, exchanges the code, and writes the resulting auth
file. CLIProxyAPI's SDK already contains the same provider implementation in
its `CodexAuthenticator`.

This code cannot be removed by simply calling the existing SDK method. The SDK
method is synchronous, prints the device code, optionally opens a browser, and
does not expose an asynchronous operation suitable for AO's UI. The clean
reduction would be an upstream CLIProxyAPI management API such as:

```text
POST   /v8/management/oauth/device/start
GET    /v8/management/oauth/device/status?id=...
DELETE /v8/management/oauth/device?id=...
```

Until such an API exists, the custom flow is duplicated implementation but
still serves a different asynchronous product contract.

## Usage fetching is only partially duplicative

[`proxy-host/internal/host/server.go`](../../proxy-host/internal/host/server.go)
has an AO-specific `/ao/account-usage` route. It uses CLIProxyAPI's authenticated
request machinery to call:

- `https://chatgpt.com/backend-api/wham/usage`
- `https://api.anthropic.com/api/oauth/usage`

CLIProxyAPI v8.0.8 exposes `/v8/management/requests/api-call`, which could
perform these authenticated calls through its public management API. That
could remove some helper-specific transport code, but AO would still need to
parse provider-specific JSON, normalize quota windows, cache results, and
return a stable product response.

The CLIProxyAPI `/v0/management/quota/fetch` endpoint is not a complete
replacement. It uses a registered plugin quota provider or a declarative probe;
it does not provide AO's normalized Codex and Claude usage model by default.

## Exact routing is not replaceable by the current public API

The product requires a session to stay on its selected account. A request from
session A must not silently run on account B because account A is unavailable,
refused the request, or lacks a model.

CLIProxyAPI exposes `handlers.WithPinnedAuthID` in its embedded SDK, but its
public HTTP management API has no request-level exact-auth parameter with a
hard no-fallback guarantee. AO therefore needs the boundary in
[`proxy-host/internal/host/server.go`](../../proxy-host/internal/host/server.go)
to translate a private session ticket into a pinned SDK request, and it needs
[`proxy-host/internal/host/routes.go`](../../proxy-host/internal/host/routes.go)
to validate tickets, track active requests, and apply route revisions safely.

The current trimming pass keeps the custom exact-account selector and does not
rely only on `WithPinnedAuthID`. Focused tests show CLIProxyAPI v8.0.8
selecting another credential after the selected credential is disabled or
refuses a request:

- `TestSDKAccountDisableAndRemovalNeverSelectAnotherAvailableCredential`
- `TestSDKRuntimeConfigRefreshRetainsAccountPinAndModelRestrictions`
- `TestClaudePublicSDKNeverFallsBackAfterProviderRefusal`

Those failures demonstrate that the selector is not redundant under AO's exact
session-account contract. It should remain until CLIProxyAPI provides a hard
pin/no-fallback guarantee or the product requirement changes.

## Why AO's account service remains necessary

[`backend/internal/service/provideraccounts/service.go`](../../backend/internal/service/provideraccounts/service.go)
owns behavior that has no equivalent in CLIProxyAPI:

- separate Codex and Claude primaries;
- AO account IDs and stable account labels;
- explicit per-session account assignment and switching;
- sign-out versus account removal;
- preserving archived-session mappings;
- changing only sessions using an exhausted Codex primary;
- blocking destructive mutations while requests are active;
- committing route changes through a durable intent;
- recovering after a helper acknowledgement or SQLite write is lost.

[`backend/internal/storage/sqlite/store/provider_accounts.go`](../../backend/internal/storage/sqlite/store/provider_accounts.go)
persists AO's durable facts and pending intents. CLIProxyAPI persists provider
credentials and configuration, but it does not know about AO sessions,
archived workers, AO defaults, or AO's mutation boundaries.

The duplicate-looking route state is deliberate: SQLite stores AO's durable
desired state, while the helper stores effective admission state and active
request leases. CLIProxyAPI has no management API for atomically applying this
AO route map with these lease semantics.

## Safe reduction plan

The following reductions are technically realistic without changing the
feature:

1. Make CLIProxyAPI's credential inventory the source of truth for credential
   existence. AO can retain only its account mapping, primary selection, and
   session routes, fetching display metadata from the inventory where safe.
2. Consider replacing `/ao/account-usage` transport with
   `/v8/management/requests/api-call`, after adding reliable auth-index mapping
   and response-shape tests.
3. Move the Codex device flow into an asynchronous CLIProxyAPI management API.
4. Add an upstream login-result response containing the exact auth ID/index so
   AO can remove some post-auth tagging and inventory-diff logic.
5. Add an upstream inference API that accepts an exact auth index and guarantees
   no fallback. This could remove the custom selector and reduce the helper
   boundary.

If the product no longer requires exact per-session account routing, the largest
reduction would be to use CLIProxyAPI's normal fill-first, round-robin, or
session-affinity routing. That would eliminate much of AO's ticket and route
machinery, but it would be a different feature.

## Final assessment

- Credential acquisition and storage are already substantially delegated.
- Provider protocol handling, token refresh, and ordinary inference are
  delegated.
- The custom Codex device flow is the clearest duplicated implementation.
- Usage transport and normalization are a partial delegation opportunity.
- Session routing, durable account state, recovery, and AO switching policy are
  not replaceable by CLIProxyAPI v8.0.8's current API.
- Removing the exact-account selector in the current worktree is unsafe because
  the focused routing tests show fallback to another credential.
