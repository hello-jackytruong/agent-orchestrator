# Final architecture and failure-case review

Review performed on 2 October 2026 against the uncommitted implementation on `codex/cliproxy-account-manager`, based on `ec1c6122a`. This review checked the implementation, the pinned CLIProxyAPI v8.0.8 source, failure tests and the earlier live Codex evidence. The fixes are in the implementation checkout; the separate running dev checkout was not restarted or updated during this review. The live app was preserved; no new provider inference, Claude login, publication or deployment was performed during this review.

Follow-up on 3 October: the reviewed fixes and a newly reported idle-sign-out correction were loaded into the user's existing dev app through a graceful daemon refresh. Native Codex last-account sign-out passed with session identities preserved. See [the follow-up evidence](idle-signout-fix.md); the dated review results below remain the record of the original review.

## Findings and fixes, by severity

### P1 — An exited worker bypassed its still-active reviewer (fixed)

`Manager.AcquireAccountMutation` returned early for exited or terminated workers before acquiring the reviewer admission guard. An exited worker can still have an active review: review admission rejects termination, but does not require the worker's agent process to be running. Between a reviewer's model requests, the helper's in-flight HTTP count can be zero while the review is still executing tools. Switching or signing out the owner's account could therefore change that review's account mid-run.

The manager now obtains reviewer admission even when the worker has stopped. It skips only the stopped worker's controller checks. The new `TestAccountManagerFencesReviewersAfterWorkerExit` failed against the previous implementation for both exited and terminated records, then passed with the fix. It checks busy refusal, idle retry, input exclusion, release exactly once, and preservation of reviewer/worker lifecycle.

Files: `backend/internal/session_manager/provider_accounts.go`, `provider_account_review_fences_test.go`.

### P2 — Missing helper identity silently created a replacement identity (fixed)

The helper client treated missing `run/host.json` as a first launch even if its routes, configuration or credential directory already existed. Generating new keys and a new port can strand existing session tickets and, if the detached old helper still runs, permit an additional helper against the same state.

The client now refuses identity creation when existing helper artifacts are present, and reports that the original identity must be restored. It does not guess process ownership, rotate keys or delete credentials. Regression cases cover each artifact and assert that no replacement manifest is written and existing content is unchanged. Normal first launch and stable reuse retain their existing coverage.

Files: `backend/internal/adapters/proxyhost/client.go`, `client_test.go`.

### P2 — A lost login attempt kept the settings panel waiting indefinitely (fixed)

Login attempts are intentionally daemon-memory state. After a daemon replacement, polling an old attempt returns `PROVIDER_LOGIN_NOT_FOUND`. The panel discarded that error code and treated it as a transient transport failure, continuing to poll and leaving Add Account disabled until the user manually cancelled.

The hook now retains the API error code. The panel ends only an explicitly unknown attempt, explains that sign-in must be repeated and enables fresh sign-in. Transport failures still preserve the current attempt and retry. The regression failed before the fix and passed afterward; it checks that fresh sign-in opens a new URL without deleting or changing existing accounts.

Files: `frontend/src/renderer/hooks/useProviderAccounts.ts`, `components/settings/ProviderAccountsSection.tsx`, `ProviderAccountsSection.test.tsx`.

### P2 — A failed busy-operation abort hid a still-pending change (fixed)

When the helper refused a routing change because it was busy, AO attempted to clear the uncommitted intent. If that SQLite write also failed, the returned error still matched the ordinary busy error. The API therefore told the user to retry later even though the durable intent remained and subsequent recovery could complete it.

The service now reports `PROVIDER_ACCOUNT_RECOVERY_REQUIRED` for this combined failure, without matching the plain busy error. A real-SQLite/local-HTTP regression failed before the fix and passed afterward. The existing unit test was updated to require recovery classification while retaining the underlying database failure. It checks unchanged committed facts and credentials, released input fences, blocked new account resolution, and both recovery outcomes after daemon replacement: completion when idle, or durable cancellation if still busy. Ordinary busy refusals with successful cleanup continue to require a fresh user action.

Files: `backend/internal/service/provideraccounts/service.go`, `sqlite_failure_test.go`.

## Architecture and behavior assessment

| Area | Code assessment and practical limit |
|---|---|
| Ownership | AO owns catalogue identities, separate provider primaries, session assignments, admission and the SQLite journal. The separate Go module imports CLIProxyAPI; the helper owns provider credentials, OAuth persistence and inference. React remains a thin API consumer. No provider SDK enters the renderer or core daemon module. |
| Integration size | Effective authored additions after review: 2,930 production lines and 12,276 test lines, or 4.19 test lines per production line. These are added source lines, excluding generated files, dependencies, comments, blanks and static translation data. This is not a complexity or coverage percentage. |
| Exact routing | A stable HMAC session ticket is hashed for helper lookup. The boundary replaces account/provider selection headers and captures the selected route for the entire request. The custom SDK selector accepts only that auth identity/provider, sets pinned-auth metadata, and rejects absent or disabled identities. Inspection of the SDK's custom-selector path and existing SDK HTTP/SSE tests supports no cross-account fallback on retries, quota refusal or unavailable models. WebSockets are disabled rather than retaining a connection-level account pin. |
| Primary changes | `SetPrimary` updates only the provider default; it does not change routes. `ResolveAccount` chooses the primary or explicit selection for a new spawn, and `AssignAccount` verifies eligibility again. A concurrently removed selection fails rather than silently choosing another account. Codex and Claude defaults are independent. |
| Busy protection | AO fences worker input, terminal handoff/drain proof, queued/running Chat turns and related reviewers. The helper atomically checks active requests when applying a whole route snapshot. The exited-worker reviewer omission is fixed above. Normally a busy refusal clears the intent and requires an explicit retry. If clearing the intent also fails, the operation remains unresolved and now reports recovery required, as described above; this is not presented as a safely cancelled action. |
| Sign-out/removal | Sign-out clears the credential reference but retains the catalogue row; removal deletes the row. Removing a primary with alternatives requires an eligible same-provider replacement. Affected routes move to the primary; unrelated routes remain. Last-account removal retains managed empty routes and fails authentication instead of using native global credentials. A subsequent verified login becomes primary and restores waiting routes with the same session tickets. |
| Mutation recovery | A serialized service mutation obtains admission, saves a CAS-protected pending intent, verifies an exact helper acknowledgement, commits facts, removes any obsolete credential, then clears the intent. Recovery replays the durable operation before another mutation. Lost acknowledgements and failed DB commits can temporarily leave the helper ahead of published AO facts; the pending intent records that uncertainty and blocks new account resolution. Recovery may wait for newly busy sessions to become idle. |
| Helper restart | The private manifest preserves endpoint and ticket key. The client probes the authenticated protocol, and restarts only a confirmed-dead owned process. Startup and maintenance replay acknowledged state. Unknown live processes, corrupt identity, missing established identity and inconsistent route revisions fail closed. Process-crash recovery is tested; arbitrary deletion of the entire proxy directory or power-loss durability across independent files is not a promise of automatic recovery. |
| OAuth | Callback listeners bind explicit loopback addresses and validate state; the helper disables the SDK's all-interface callback forwarder. AO verifies the exact login tag before recording an account. Deleted/mismatched catalogue targets are refused and unreferenced rejected credentials are cleaned up. A daemon restart requires a fresh login attempt; it does not silently adopt orphaned SDK credentials. The UI now handles this case. |
| Credentials and management | Session processes receive session tickets, not upstream refresh tokens or management keys. Private control requests require a separate key and management allowlist. AO account routes reject unapproved browser origins and are blocked on the LAN listener. The primary AO listener retains its existing unauthenticated loopback contract. This is request-level isolation, not an OS sandbox against an arbitrary same-user process reading private local files. |
| Storage/migrations | Migration 173 introduces only the account snapshot/journal and trigger-generated CDC. It preserves the known earlier preview histories that consumed 171/172. Existing migration 140 is unchanged. Store validation checks provider/account/route referential consistency and malformed state before use. |
| Prepared creation | Account resolution uses the final spawn configuration, not the initially unresolved prepared record. Existing regression cases cover both providers, opposite project defaults, synchronous/asynchronous promotion, launch environment and single-controller behavior. |
| Scope limits | Existing native sessions stay native. Managed sessions cannot change provider in place; create another session for another provider. Same-provider reviewers share their owner's route; another-provider reviewers use their existing native behavior. CLIProxyAPI's complete management UI and every upstream feature are not exposed by this deliberately scoped AO panel. |

The failure tests are substantive: they include real SQLite journals and injected write failures, local HTTP acknowledgements, actual SDK selection paths with fake executors, request-lifetime concurrency, subprocess identity/restart fixtures and user-visible UI journeys. The line ratio alone did not establish completeness: the exited-worker reviewer gap demonstrates why review of the boundaries was still necessary.

## Investigation of the two documented gaps

**Standalone creation: existing dev database inconsistency, not an account integration regression.** Read-only inspection found `sessions.project_id` marked NOT NULL in both the current dev database and the immutable online backup made before the integration compatibility fixes. Both record migration 140 as applied; the backup's latest migration is 172. Main's unchanged migration 140 explicitly makes this column nullable and checks the postcondition. Account migration 173 does not alter it. The failure occurs at session insertion, before account assignment/inference. This proves the inconsistency predates migration 173; it does not establish which earlier preview/manual migration caused it. The user's database was not rewritten to repair this unrelated history. Standalone creation on that database remains unverified and needs a separate, backed-up schema repair.

**Native controls: partial verification remains.** The same Electron main process (PID 73413) was kept running. Its Account Manager visibly showed both real Codex accounts, A primary with three dummy sessions, B with none. The existing daemon independently reported the same state with no recovery pending. Native automation intermittently returned an unchanged/incomplete accessibility tree or `noWindowsAvailable`, including on ordinary Settings navigation; rebinding the running application restored page visibility. A delayed screenshot showed that navigation had succeeded despite the unchanged tree. The primary sign-out confirmation could not be reliably exercised through these controls in this pass. Source inspection and component tests verify the handlers and replacement selection, but do not prove every native button works. No app defect was confirmed from this automation failure, and the native click-through gap is retained explicitly.

The earlier [live verification](codex-e2e-verification.md) remains the evidence for two-account GPT-5.5 Low inference, busy refusals, primary/new-session behavior, idle switching and last-account recovery without restarting the Codex conversation. This review did not repeat destructive account operations or spend additional model requests.

## Validation

Newly executed checks (heavy suites run sequentially):

| Check | Result |
|---|---|
| Exited-worker reviewer regression | Failed before the fix for both worker states; passed afterward with race detection |
| Combined busy refusal / failed journal abort regression | Failed before the fix for both recovery outcomes; passed afterward with race detection |
| Focused Account Manager and account hooks | 44 tests passed; unknown-login regression failed before the fix |
| Full session-manager race suite | Passed, 145.767 seconds |
| Full reviewer race suite | Passed, 14.056 seconds |
| Full account-service race suite after final fix | Passed, 25.667 seconds; includes real-SQLite busy/abort failure and both recovery outcomes |
| Full helper-client race suite after identity fix | Passed, 10.300 seconds |
| Full separate SDK helper race suite | Passed, 8.821 seconds |
| Full frontend suite, one worker | 362 files passed; 5,828 tests passed, 6 skipped; 520.14 seconds |
| Frontend typecheck and renderer build | Passed; existing large-chunk warning only |
| Backend build/vet/pinned lint | Passed again after the final service fix; lint reports 0 issues |
| Working-tree whitespace check | Passed |

The final review did not edit migrations or generated API contracts. The earlier complete SQLite race suite (677.035 seconds), full backend package results and generated-artifact checks remain in [implementation progress](implementation-progress.md); they are not relabelled as newly executed review checks. Local review logs use the `/tmp/ao-review-` prefix.

## Readiness

**Ready for PR/CI validation after the four review fixes; not yet release-certified.** No unresolved P0/P1 defect was identified in the scoped review. The affected backend race suites, complete frontend suite, helper race suite, builds, typecheck, vet and pinned lint passed. The running Electron app still uses its pre-review build: these fixes need to be loaded there before claiming live verification of the final code. Preserve that app and its data when arranging the follow-up. Native Windows execution, signed release packaging/notarization, Docker external smoke checks and remote CI remain outside local evidence. Real Claude, compacted/tool-heavy cross-account continuation and live provider quota/outage scenarios remain unverified. No publish, deployment or PR was created.

## Subsequent native UI finding — 3 October

**[P3, fixed] Account-switch notices outlived the route they described.** Native Chat displayed an earlier busy-switch refusal after account removal changed its route to login-required. The notice now carries its owning session/account and is discarded when that route changes, including when a delayed refusal arrives after an external account-management operation. Four focused regressions were added; three first reproduced the defect before the fix. The final component/journey run passed 18 tests and frontend typecheck passed. The component was updated in the existing dev checkout without restarting Electron; a subsequent native screenshot confirmed the obsolete notice was absent from the login-required session. Signed-in variants remain blocked by browser access.

The [latest Codex native matrix](codex-ui-verification.md) supersedes the earlier native-control checkpoint: 48 verified, five partial and 11 blocked/skipped. The Playwright continuation added partial live daemon/login/helper recovery evidence, preserved all 40 session records and native runtime/conversation identities, and restored the normal Electron launch without the temporary debugging port. Chat controllers were recreated on daemon restart as expected. The user explicitly authorized skipping unsafe/blocked actions. Saved browser permissions block automated OpenAI OAuth recovery, and the last-account test currently leaves the Codex catalogue empty. Sessions are preserved and waiting for sign-in; this cleanup dependency prevents claiming a completed live recovery pass. Automated tests are not substituted for skipped native cases. Effective authored counts are 2,935 production / 12,439 test lines (4.24:1).

## Latest Codex UI continuation — 4 October 2026

The [current UI matrix](codex-ui-verification.md) supersedes older permission-blocked checkpoints: **62 verified, one partial, one not run live**. Browser access works and both Codex accounts are restored; A is primary and scratch-9 is explicitly pinned to B. Thirteen of the 15 previously remaining cases are newly complete. Stale callback redelivery against a fresh listener remains partially blocked by Chrome (`ERR_BLOCKED_BY_CLIENT`); the old closed relay admitted no account and fresh login succeeded. The live reviewer test was withheld because AO’s reviewer prompt requires publishing a GitHub review, outside the explicit no-publish scope; reviewer protection passed the sequential race suite.

Two additional defects were fixed: Codex terminal idle proof now understands both current `›`/`»` glyphs while preserving style/draft/approval safeguards, and the default new-task choice no longer sends a cached primary as an explicit pin. Live open-form creation reproduced stale A before the fix and selected current A after a B-to-A primary change following the fix. Explicit choices remain explicit. Thirty-four glyph cases and four default-resolution regressions demonstrate before/after behavior; 88 complete frontend component/journey tests and the affected backend race suites passed sequentially. Counts are **2,963 authored production / 12,576 test lines (4.24:1)**. Signed-in daemon/helper recovery, real approval-wait protection, primary removal and controlled missing-account isolation have actual Electron UI evidence. All inference used GPT-5.5 Low; Claude and original conversations were preserved. See the matrix for controlled-fixture distinctions and final cleanup/validation details.

Final cleanup restored both Codex sign-ins with A primary and all nine managed routes on A, including the disposable-session pins. The same app/profile/data runs normally with no port 9337 or test Chromium flags. Original 38 session native identities/termination facts and Claude state are preserved; 42 total records include four test sessions. Final frontend typecheck, backend build/vet and diff checks passed. The two live evidence gaps above remain; no publication/deployment occurred.

### Additional severity-ranked findings from the final UI continuation

1. **P1 — Cached default could route a newly created session to the previous primary (fixed).** With the real creation form open, a primary change made B the default but submission explicitly sent cached A. The frontend now sends an ID only when the user explicitly chooses an account; default resolution happens in the daemon at admission. Before/after live creation and four failing-before default-resolution regressions establish the fix.
2. **P2 — Existing Codex terminal compatibility gap prevented safe idle operations and return-to-Chat (fixed).** Codex 0.160 rendered `»` while the main-branch detector recognised only `›`. The adapter now recognises the current glyph without changing draft text or style-sensitive proof. Thirty-four compatibility cases preserve active, draft and approval safeguards; actual signed-in handoffs succeed. This is an existing adapter issue exposed by the integration, rather than loss of session-account routing.
3. **Validation limits, no confirmed remaining integration defect:** live reviewer protection requires a nonpublishing fixture because the existing reviewer workflow posts GitHub reviews; stale callback redelivery against the fresh listener was blocked inside Chrome. The actual old closed relay admitted no account, and fresh sign-in succeeded. Both limits remain separate from the passing automated reviewer/callback safeguards.

Readiness: suitable for final architecture review with those two declared live evidence limits; not full live certification of all 64 cases. Current authored production scope remains below 3,000 effective lines and meets the minimum 4:1 test-line requirement.
