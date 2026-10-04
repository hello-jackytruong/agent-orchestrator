# Live Codex verification

Completed 2 October 2026 in the user's already-running **Agent Orchestrator (dev)** Electron app. This pass covers Codex only; every real dummy session and model request used **GPT-5.5 with Low reasoning**. The subsequent [final architecture/failure-case review](final-review.md) records additional fixes and validation.

## Environment and preservation

The implementation branch is `codex/cliproxy-account-manager`, based on `main` at `ec1c6122a`. The existing app runs from `/Users/adilshaikh/.codex/worktrees/cliproxy-baseline/reverb`; the two fixes below were applied there as well as to the implementation checkout. Its renderer is `localhost:5173`, daemon `127.0.0.1:3002`, and data/profile/run state live beneath `~/.ao/dev`. No second app was launched. The daemon was gracefully restarted to load backend fixes; the existing Electron main process and detached helper were retained.

An online SQLite backup was saved beneath the existing AO data directory before compatibility changes. All **32 original session IDs remain present**. Three clearly named dummy sessions were added under the existing Scratch project: `scratch-3` and `scratch-4` in terminal mode, and `scratch-5` in Chat. Older native sessions were not converted or rerouted. Both real Codex accounts were authorized through their existing Chrome sign-ins; account A and B below are labels, not saved credentials.

## Executed cases

| Case | Action surface | Observed result |
|---|---|---|
| Add two Codex accounts | Electron login flow and Chrome OAuth | Both sign-ins completed; first account A became primary automatically |
| Start on primary A | Electron new-task form | Terminal A returned `AO_CODEX_ACCOUNT_A_OK` |
| Change primary to B | Electron Account Manager | Existing terminal retained A; next new-task form selected primary B |
| Start on primary B | Electron new-task form | Terminal B returned `AO_CODEX_ACCOUNT_B_OK` |
| Explicit A while B is primary | Existing daemon API, Chat displayed in Electron | Chat ran on A, with GPT-5.5 Low settings |
| Switch existing terminal to B | Existing daemon API; reply observed in Electron | Same terminal generation recalled `AO_ROUTE_ORCHID_7319`, originally supplied on A |
| Switch existing Chat to B | Existing daemon API; reply observed in Electron | Same conversation ID recalled `AO_CHAT_MAPLE_9421`, originally supplied on A |
| Switch/sign out during a live Chat turn | Existing daemon API | Both refused with `PROVIDER_ACCOUNT_IN_USE`; active work continued |
| Sign out primary without replacement | Existing daemon API | Refused with `PROVIDER_PRIMARY_REQUIRED` while another signed-in account existed |
| Sign out non-primary A | Existing daemon API | Signed-out row retained; its managed session moved to B |
| Sign out last account B | Existing daemon API | All three managed routes required login; existing Chat request failed without a model reply or global-login fallback |
| Spawn with no managed account | Existing daemon API | Refused with `PROVIDER_LOGIN_REQUIRED`; no new visible session created |
| Sign A in again | Existing daemon login start and Chrome OAuth | Same AO account identity became primary and restored all three waiting routes |
| Continue after re-login | Existing daemon API | Same already-running Chat recalled its original phrase; conversation ID unchanged, no session restart |
| Remove and re-add signed-out B | Existing daemon API and Chrome OAuth | Catalogue removal succeeded; B added again with a new catalogue ID; A remained primary |
| Final catalogue | Native Electron screenshot and accessibility view, cross-checked with API | Both signed in; A primary with three sessions, B with zero; Claude has no signed-in account |
| Preservation | Read-only API comparison with baseline | All 32 original IDs preserved; 35 sessions total; original dummy terminal generation and Chat conversation ID unchanged |

This combines real native-agent inference, native Electron observation and direct operations on the app's existing daemon. It is not a claim that every native button path was successfully automated. Account selector and confirmation controls were intermittent under native automation; affected operations used the documented AO API. The final catalogue view refreshed and matched persisted state.

## Defects found and corrected

**Account Manager HTTP 500.** Earlier unmerged previews had consumed migration numbers 171 and 172 without creating the account-state table. The unmerged account migration now uses 173 and tolerates already-existing account objects. Upgrade tests cover main, earlier previews and an earlier account-state schema while preserving existing facts. The complete SQLite race suite passed after the fix (677.035 seconds).

**Prepared task account assignment.** A prepared record has an unresolved provider until promotion. Assignment used that empty record field instead of the final agent selected by the user. Assignment now uses the resolved spawn configuration. Regression tests cover both providers, an opposite project default, and synchronous/asynchronous prepared launches. The complete session-manager race suite passed after the fix (251.850 seconds).

After these changes, backend build, vet and pinned golangci-lint v2.13.2 passed; generated sqlc output had no drift. Earlier complete backend/helper and frontend results are recorded in [implementation progress](implementation-progress.md). Heavy suites ran sequentially.

Effective authored additions at completion: **2,917 production lines and 12,092 test lines**, or **4.15 test lines per production line**. Generated code, dependencies, comments, blanks and static data are excluded.

## Limits and final state

- Standalone creation failed on the existing dev database's pre-existing `sessions.project_id` non-null constraint before inference. Live tests therefore used the existing Scratch project; standalone creation on this database is not certified by this pass.
- Live Codex used the installed CLI 0.156.1; automated protocol regressions use the checked-in 0.146.0 version. Passing simple text continuation does not establish every version or encrypted/compacted/tool-heavy continuation state.
- Real Claude sign-in/inference, real quota exhaustion, provider outages and all release runners remain untested. Automated failure cases do not replace those live checks.
- The app remains open. Both accounts are signed in, A is primary, and the three dummy sessions retain their routes for inspection. No PR, release or deployment was created.

Local screenshot evidence: `/tmp/ao-codex-e2e-account-a-response.png`, `/tmp/ao-codex-e2e-account-b-response.png`, `/tmp/ao-codex-e2e-switched-memory.png`, and `/tmp/ao-codex-e2e-restored-accounts.png`. These contain live account display details and remain local rather than being committed.

## Latest Codex UI continuation — 4 October 2026

The [current UI matrix](codex-ui-verification.md) supersedes older permission-blocked checkpoints: **62 verified, one partial, one not run live**. Browser access works and both Codex accounts are restored; A is primary and scratch-9 is explicitly pinned to B. Thirteen of the 15 previously remaining cases are newly complete. Stale callback redelivery against a fresh listener remains partially blocked by Chrome (`ERR_BLOCKED_BY_CLIENT`); the old closed relay admitted no account and fresh login succeeded. The live reviewer test was withheld because AO’s reviewer prompt requires publishing a GitHub review, outside the explicit no-publish scope; reviewer protection passed the sequential race suite.

Two additional defects were fixed: Codex terminal idle proof now understands both current `›`/`»` glyphs while preserving style/draft/approval safeguards, and the default new-task choice no longer sends a cached primary as an explicit pin. Live open-form creation reproduced stale A before the fix and selected current A after a B-to-A primary change following the fix. Explicit choices remain explicit. Thirty-four glyph cases and four default-resolution regressions demonstrate before/after behavior; 88 complete frontend component/journey tests and the affected backend race suites passed sequentially. Counts are **2,963 authored production / 12,576 test lines (4.24:1)**. Signed-in daemon/helper recovery, real approval-wait protection, primary removal and controlled missing-account isolation have actual Electron UI evidence. All inference used GPT-5.5 Low; Claude and original conversations were preserved. See the matrix for controlled-fixture distinctions and final cleanup/validation details.

Final cleanup restored both Codex sign-ins with A primary and all nine managed routes on A, including the disposable-session pins. The same app/profile/data runs normally with no port 9337 or test Chromium flags. Original 38 session native identities/termination facts and Claude state are preserved; 42 total records include four test sessions. Final frontend typecheck, backend build/vet and diff checks passed. The two live evidence gaps above remain; no publication/deployment occurred.
