# Idle Codex session blocking sign-out — 3 October 2026

The user reported last-account Codex sign-out refused with `PROVIDER_ACCOUNT_IN_USE` while all assigned sessions were idle. Read-only inspection of the existing Electron app confirmed five assigned sessions: two terminated terminals, one completed/idle Chat and two idle live terminals. No pending account journal or running/queued Chat turn was present.

The current styled viewport of `scratch-7` positively identified the bug. Codex 0.156.1 renders its model/directory footer followed by a separate `? for shortcuts … ⚠ 3 warnings · f2 to view` row. `codexComposerFrame` selected the final middle-dot row as the footer boundary, included the model/directory row in the composer, and classified it as a human draft. Account admission reused the interface-transition drain proof and translated the draft refusal into the generic busy error. `scratch-6`, whose warnings stayed on the model row, parsed correctly.

The fix excludes only the known separate warning/help footer row when selecting the composer boundary. Inline warnings, unsent text, wrapped text containing middle dots, active work and approval protection retain their existing rules. The busy guard was not bypassed and the database was not rewritten.

Regression evidence: the adapter regression failed before the fix for all separate warning-row variants. The real Codex adapter is also exercised through `Manager.AcquireAccountMutation`, checking repeated idle proof, busy refusal, input fencing, idempotent release and unchanged native lifecycle/history.

The implementation remains on `codex/cliproxy-account-manager`. Live verification used only the existing Electron checkout at `/Users/adilshaikh/.codex/worktrees/cliproxy-baseline/reverb`, with its current data/profile under `~/.ao/dev`. An online SQLite backup was made at `~/.ao/dev/data/backups/before-idle-signout-fix-20261003T064351Z.db`. The preservation baseline recorded 37 visible session identities and both provider catalogue entries. Yesterday's four reviewed fixes were also synced into that checkout. A graceful daemon refresh loaded the changes; the existing Electron main process and detached proxy helper were retained.

## Validation and live result

The full Codex adapter race suite passed (2.153 seconds), followed sequentially by the full session-manager race suite (221.470 seconds), backend build, vet and pinned golangci-lint v2.13.2 (zero issues). `git diff --check` passed. The updated effective authored count is 2,932 production lines and 12,385 test lines (4.22:1).

Native Account Manager sign-out succeeded for the only signed-in Codex account. The UI displayed `Signed out · 0 sessions`, the no-signed-in-Codex message and the successful account-update message. The daemon returned `signedIn: false`, an empty Codex primary and `recoveryRequired: false`. All five durable Codex routes (`scratch-3` through `scratch-7`) now have an empty account assignment, waiting for the next sign-in; the catalogue entry remains and its credential references are cleared. No Claude account operation or model inference was performed.

All 38 database session records, including the prepared record outside the visible session list, remained present. Native runtime launch IDs, conversation IDs and termination facts were unchanged. The daemon refresh reattached Chat `scratch-5` with a new AO controller generation while preserving its native conversation ID; the account mutation did not restart its conversation. No pending account journal remains.

The renderer temporarily showed the startup screen after the daemon refresh, then reconnected before the native button test. Native accessibility controls were intermittent until the existing window was raised; unlike the earlier partial E2E evidence, this specific sign-out was confirmed through the real button and visible result. The app remains open with Codex signed out and its sessions preserved. A new sign-in is required to resume managed Codex requests. This regression is fixed; the broader Claude and release-platform validation gaps from the final review remain.
