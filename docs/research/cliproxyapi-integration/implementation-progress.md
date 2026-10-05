# Account manager implementation progress

Implementation and local regression verification are complete on `codex/cliproxy-account-manager`, based on main `ec1c6122a`. Real Codex end-to-end verification in the user's existing dev Electron app is complete for the cases in the [live verification record](codex-e2e-verification.md). The final architecture/failure-case review has now been performed; see [findings, fixes and validation](final-review.md). This is not a release certification.

The helper imports CLIProxyAPI v8.0.8 in a separate Go module. It listens on loopback, authenticates private management calls, validates per-session tickets, and installs an exact-account selector. AO owns provider primaries, session choices, idle admission and the thin API/UI. Existing native sessions retain their current login.

AO stores a small account catalogue and route snapshot in one SQLite singleton, with a compare-and-swap revision and a durable pending intent. This differs from the plan's possible normalized records: a snapshot keeps the whole small catalogue update atomic. The helper stores its acknowledged snapshot separately. Route acknowledgement precedes committing AO facts; credentials are removed only after reassignment. Busy refusals clear an uncommitted intent, requiring a user retry.

All files live beneath the resolved AO data directory: `proxy/auth` for upstream credentials, `proxy/config.yaml`, `proxy/run/host.json` for the private process identity and keys, `proxy/run/routes.json` for acknowledged routes, and `proxy/logs/host.log`. Native conversation storage remains in AO's existing native session directories.

Packaged daemons find the helper beside their executable. macOS/Linux desktop development runs the daemon through `go run`, whose executable is temporary; the desktop supplies the absolute built helper path through `AO_PROXY_HOST_BINARY`. Configuration rejects a relative override. Windows development continues discovering the sibling beside its versioned daemon executable. A live SDK login-start probe found and corrected this development-path gap. The real SDK also requires a configured secret before accepting its local management password; the helper supplies a bcrypt hash of its private control key, without enabling remote management or saving that key in plaintext. Complete helper race tests and both live login-start flows passed after these fixes.

Daemon startup and a ten-second maintenance check replay the current acknowledged routes. If the owned helper has definitely stopped, the client restarts it on the same endpoint using its existing private identity, so running agent processes retain their session tickets. Unknown or incompatible live processes are never replaced. Missing or inconsistent routing storage fails closed rather than selecting another account.

Focused fake-provider and HTTP boundary checks have passed. They cover exact Codex and Claude account selection, ordinary streaming, management separation, account rules, native launch arguments, OAuth callback relays, detached helper reuse/restart, reviewer and worker input fences, and settings actions. Cross-boundary checks use the real account service, SQLite journal, and helper HTTP client against a local protocol peer. They cover lost acknowledgements, malformed acknowledgements, failed SQL writes, credential cleanup, daemon replacement, and busy refusals. These automated checks use fake credentials. Subsequent user-authorized testing has signed two real Codex accounts into the existing dev app; no real Claude account testing is being performed.

The complete backend race run passed every package except a new chat test that read the completed message before the live turn released its admission lock. After correcting that lifecycle wait, the complete chat-service race suite passed too. Complete affected API/account-service and config/daemon race suites were rerun after subsequent fixes. Every backend package has a passing result, including SQLite migration, storage, session-manager, account-service, daemon and API suites. Heavy suites ran sequentially.

| Local check | Final result |
|---|---|
| Backend race suites | All packages verified, with complete affected-package reruns as described above |
| Separate SDK helper race suite | Passed after the management-secret fix, including real SDK management/login-start coverage |
| Complete frontend suite, one worker | 362 files; 5,827 passed, 6 skipped |
| Frontend TypeScript and renderer build | Passed |
| Backend and helper build/vet | Passed |
| Pinned golangci-lint v2.13.2 | Passed, no issues |
| sqlc and OpenAPI/TypeScript regeneration | Passed, no generated drift |
| Desktop ACP runtime and daemon/helper bundle | Passed in an isolated worktree with independently installed dependencies |
| Windows backend/helper cross-builds | Passed; native Windows execution not tested |
| Real Electron settings | Account Manager, separate Codex/Claude sections and retained Subscriptions verified |
| Actual SDK through the scratch desktop daemon | Both providers: HTTPS login link generated, pending status checked, attempt cancelled; catalogue unchanged |

Early frontend failures came from request fixtures and incompatible local native/browser dependencies; fixes separate the new account requests and verification uses the pinned browser executable plus a rebuilt SQLite dependency. The installed Codex 0.156.1 differs from the checked-in 0.146.0 protocol; backend regression checks used a temporary pinned CLI without modifying the global install. The desktop lab used a detached main-based worktree containing the authored changes, scratch data/profile/run file under `/tmp/ao-account-desktop-final-20261002`, daemon `127.0.0.1:33427` and renderer `localhost:5173`. That isolated lab did not add real provider credentials. The subsequent live Codex pass used the user-requested existing dev app and is recorded separately below.

The final count is 2,917 effective authored production lines and 12,092 effective authored test lines (4.15 test lines per production line), meeting the requested minimum of four test lines per production line. This counts added authored lines; generated files, dependencies, comments, blanks and static data (including translated message catalogues) do not contribute to this ratio.

Real Codex terminal and Chat conversations continued after an idle account switch, recalling a phrase supplied on the first account. The existing Chat also recovered after the last account was signed out and signed in again, without restarting its conversation. This establishes the tested GPT-5.5 Low text flows on Codex 0.156.1, not universal portability of encrypted/provider-owned state. Real Claude continuation, tool-heavy/compacted conversations and quota failures remain unverified.

The owned scratch Electron, daemon and helper processes were stopped and the scratch daemon listener was confirmed closed. The isolated worktree remains available for the final review.

Native Windows/macOS release packaging, signing/notarization, Docker-based external smoke checks and remote CI were not run. No PR, publish or deployment was created. The local Electron bundle check is not a signed-release validation.

## Existing dev app: live Codex verification completed

The user requested the already-running dev Electron app, preserving its data under `~/.ao/dev/data`. The original 32 session IDs were recorded and an online SQLite backup was saved beneath that data directory before compatibility changes. No existing session was rerouted. Every real dummy session uses GPT-5.5 with Low reasoning.

The live test found that earlier account-manager previews had already applied migration numbers 171 and 172 without creating `provider_account_state`, causing the Account Manager HTTP 500. The unmerged feature migration now uses 173 and tolerates an already-existing table, seed row, and trigger. Upgrade tests preserve existing session and routing facts across those preview histories. The complete SQLite race suite passed after this change (677.035 seconds).

The next live test found that a prepared task record has no resolved provider yet. Account assignment incorrectly used that unresolved field before promoting the preparation, rather than the final agent selected in the form. Assignment now uses the resolved spawn configuration. Regression cases cover both providers and synchronous/asynchronous prepared launches. The focused race check passed; the complete session-manager race suite passed (251.850 seconds).

Both real Codex logins succeeded through the existing Chrome accounts. The first became primary automatically. A live terminal session on account A replied `AO_CODEX_ACCOUNT_A_OK`. Changing the primary to B left that existing route on A, and the next new session defaulted to B and replied `AO_CODEX_ACCOUNT_B_OK`. The terminal recalled its original phrase after switching to B with the same terminal generation. A Chat explicitly assigned to A while B was primary also continued after switching to B with the same conversation ID. Busy switch/sign-out attempts were refused. Signing out A reassigned its sessions to B; signing out the last account made managed requests unauthorized, and a new managed spawn was rejected. Signing A in again restored all three routes and the same Chat answered its original memory check without a restart. The signed-out B catalogue row was removed and B was added again; A remained primary. Both accounts are signed in at completion.

The final native Account Manager view shows both signed in, A primary with three sessions, and B with zero. Native account selectors/dialog controls were intermittent under automation, so several operations were verified through the existing daemon API rather than claimed as successful button tests. The pre-existing dev database also rejects standalone session creation because `sessions.project_id` is non-nullable; live sessions used the existing Scratch project. See the [case-by-case record](codex-e2e-verification.md) for these limits. The user's app and its three clearly named dummy sessions remain open; all 32 original session IDs are preserved.

## Final review follow-up

The final review found and fixed four additional defects: stopped workers bypassed their active reviewers during account mutation; a missing helper identity could be regenerated despite established helper state; a daemon restart left the UI polling an unknown login attempt indefinitely; and a helper busy refusal combined with failed SQLite cleanup hid an unresolved pending operation. Regression tests cover each boundary. Full affected backend race suites and the separate SDK helper race suite passed sequentially; final frontend/build checks are recorded in [the review report](final-review.md).

The updated effective authored count is 2,930 production lines and 12,276 test lines (4.19:1). The earlier 2,917/12,092 count above records the end of the live E2E pass. Read-only inspection of the pre-fix backup confirmed that the standalone project-ID constraint already existed before migration 173. Native control automation remains a partial-validation gap.

## Idle sign-out regression — 3 October 2026

The user's idle Codex sign-out report exposed a separate warning footer being parsed as an unsent draft. A narrowly scoped composer-boundary fix and adapter/manager regressions preserve active-work and real-draft protection. Full affected race suites, build, vet and lint passed sequentially. Native last-account sign-out now succeeds in the existing Electron app; all session records and native conversation identities are preserved. The account remains in the catalogue as signed out, and its five routes wait for sign-in. Updated effective counts: 2,932 production and 12,385 test lines (4.22:1). See [the diagnosis and live verification](idle-signout-fix.md).

### Native UI continuation and stale-notice fix — 3 October

The Codex native matrix has 48 verified cases, two partial and 14 skipped after the user instructed skipping unsafe/blocked actions. New native evidence includes explicit Chat creation, conversation continuation after account switching, one-busy-member account protection, primary change during work, idle Chat/Terminal removal and last-account preservation. See [the current matrix and exact gaps](codex-ui-verification.md).

One additional display defect was fixed: account-switch notices were retained after account management changed the session's route. Notices now belong to the session/account they describe and are discarded after route changes. Four focused regressions were added; the final component/journey run passed 18 tests, frontend typecheck passed, and the full local `npm run build` / Electron Forge darwin-arm64 packaging passed. Native post-fix verification is blocked by the locked Mac. Effective authored counts: 2,935 production and 12,439 test lines (4.24:1).

The live Codex catalogue currently has no signed-in account after the last-account removal test. Recovery through Chrome is blocked by its saved permission for `auth.openai.com`; no credential or browser workaround was used. Original sessions are preserved and waiting for sign-in. This remaining cleanup dependency must not be described as successful recovery.


### Playwright/recovery continuation — 3 October

With explicit Playwright authorisation, the same existing Electron app was attached temporarily through loopback debugging. Real daemon restarts preserved all 40 records and native runtime/conversation identities; the two Chat controller generations changed as expected. A pending login became unknown after restart. Controlled helper termination/replacement also preserved catalogue/defaults and waiting routes; its temporary login was cancelled. These are partial boundary checks, not signed-in UI passes. The normal app launch was restored and debug port 9337 removed. Current matrix: 48 verified, five partial, 11 blocked/skipped. Chrome still rejects `auth.openai.com` under its saved permission and the native tool still reports a locked Mac. No production edit, extra inference, Claude operation or heavy suite rerun occurred. See [the exact evidence and remaining gaps](codex-ui-verification.md).

## Latest Codex UI continuation — 4 October 2026

The [current UI matrix](codex-ui-verification.md) supersedes older permission-blocked checkpoints: **62 verified, one partial, one not run live**. Browser access works and both Codex accounts are restored; A is primary and scratch-9 is explicitly pinned to B. Thirteen of the 15 previously remaining cases are newly complete. Stale callback redelivery against a fresh listener remains partially blocked by Chrome (`ERR_BLOCKED_BY_CLIENT`); the old closed relay admitted no account and fresh login succeeded. The live reviewer test was withheld because AO’s reviewer prompt requires publishing a GitHub review, outside the explicit no-publish scope; reviewer protection passed the sequential race suite.

Two additional defects were fixed: Codex terminal idle proof now understands both current `›`/`»` glyphs while preserving style/draft/approval safeguards, and the default new-task choice no longer sends a cached primary as an explicit pin. Live open-form creation reproduced stale A before the fix and selected current A after a B-to-A primary change following the fix. Explicit choices remain explicit. Thirty-four glyph cases and four default-resolution regressions demonstrate before/after behavior; 88 complete frontend component/journey tests and the affected backend race suites passed sequentially. Counts are **2,963 authored production / 12,576 test lines (4.24:1)**. Signed-in daemon/helper recovery, real approval-wait protection, primary removal and controlled missing-account isolation have actual Electron UI evidence. All inference used GPT-5.5 Low; Claude and original conversations were preserved. See the matrix for controlled-fixture distinctions and final cleanup/validation details.

Final cleanup restored both Codex sign-ins with A primary and all nine managed routes on A, including the disposable-session pins. The same app/profile/data runs normally with no port 9337 or test Chromium flags. Original 38 session native identities/termination facts and Claude state are preserved; 42 total records include four test sessions. Final frontend typecheck, backend build/vet and diff checks passed. The two live evidence gaps above remain; no publication/deployment occurred.

## Codex request-boundary experiment — 4 October 2026

Committed the previous implementation locally as `909987581` before changing behavior. The new experiment remains uncommitted for separate review and is enabled only by `AO_CODEX_REQUEST_ACCOUNT_SWITCHING=1` at daemon startup. Codex primary A→B now moves all managed routes currently on A (including explicit A choices) for the next HTTP request; individual Codex switches also work while busy. Accepted requests and internal SDK retries retain their selected account. Claude and disabled-mode behavior stay at the checkpoint.

The implementation adds the signed-in auth inventory to private route snapshots and outstanding account leases to the helper, retaining safe sign-out/removal after a session has moved. Durable intent records preserve the original admission mode across recovery; helper protocol 2 rejects incompatible running helpers. Old route files can bootstrap their initial inventory without changing routes or revision. See [the verification record](request-switching-verification.md) for mechanics, test cases, failures and evidence boundaries.

Validation passed sequentially: affected backend race suites; full helper race suite plus 20 repeated HTTP/SSE concurrency cases; 131 complete frontend component/journey tests with one worker; frontend typecheck; backend build/vet; generated API regeneration and diff checks. A test assertion incorrectly assumed concurrent responses finish in admission order; it was corrected, and the affected complete suite passed again.

Actual Electron tests used only disposable `scratch-9`, GPT-5.5 Low, the existing profile/data and two existing Chrome sign-ins. Primary switching during a tool turn, individual switching during approval wait and continuation after A compaction on B all succeeded with local selected-auth evidence and unchanged conversation/controller identities. Denial cancelled its command and turn by existing Codex behavior; the next explicit message used the switched account. No live quota exhaustion/provider outage was induced; unchanged previous missing-target evidence and new SDK failure-after-rebind regressions are distinguished from live cases.

Added authored code in the experiment: 97 production and 801 test lines, excluding generated/static/dependency artifacts, comments and blanks. Whole branch versus `ec1c6122a`: **3,029 production / 13,364 test lines (4.41:1)**; this is a net increase of 66 production lines over the checkpoint, because some additions replace previous implementation lines.

Cleanup restored the actual starting catalogue exactly: Codex B primary/signed in with nine routes, A retained/signed out with zero, Claude unchanged. All 42 native conversation identities/termination facts were preserved. The existing Electron app is running its original normal dev sources/binaries with the same data/profile, no experimental/debug flags, no port 9337 and no temporary selector tracing. The new experiment is ready for its separate architecture/failure-case review, not enabled by default or published.

## Codex quota primary recovery — 4 October 2026

Added an opt-in Codex quota auto-switch control to Account Manager. CLIProxyAPI's result policy records only the explicit `usage_limit_reached` signal, including its reset hint, in a private helper sidecar. AO requires two signed-in Codex accounts before enabling the setting; with fewer, the visible control is disabled with an information hint. When enabled, AO polls those events, confirms the reported auth is still the current primary, selects the first other signed-in Codex account without checking its quota, changes the primary, and rebinds every route that currently used the old primary. Other-account sessions remain unchanged. A primary-generation marker rejects delayed A→B→A events. Rate limits, outages, auth failures, and Claude results do not trigger this path. See [quota-auto-switch.md](quota-auto-switch.md).

Focused service, controller, helper, client, and frontend tests pass. Full heavy suites and live Electron verification remain to be run after this change; no real quota exhaustion was induced.

## Additional account acquisition methods — 5 October 2026

The Account Manager now exposes the four CLIProxy-compatible ways to add an account: browser sign-in, Codex device-code sign-in, API-key entry, and credential-JSON import. AO still owns the account catalogue and routing rules; the helper owns provider-specific exchange, credential-file/config writes, and SDK inventory confirmation. Secrets stay in the helper's private AO data directory and are never returned to the renderer. Device login is Codex-only, while API keys and JSON imports support both provider sections. The UI reports a safe label, provider, kind, and status and never displays a token or API key.

The new helper boundary has focused coverage for input validation, provider response failures, device polling, import filtering and size limits, private file permissions, API-key persistence/removal, duplicate operations, and safe result serialization. Heavy suites and live sign-in were intentionally not rerun after the user stopped the local Node processes; only formatting and diff checks were performed in this follow-up.
