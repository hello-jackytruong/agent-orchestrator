# Planning decisions and question queue

Last updated: 2 October 2026.

Purpose: preserve the product decisions settled before implementation. All questions below are answered. The user subsequently authorized implementation and tests; current verification is tracked in [implementation progress](implementation-progress.md).

## Decisions confirmed by the user

| Topic | Confirmed decision |
| --- | --- |
| Architecture | Approach 1: one detached AO-owned helper importing CLIProxyAPI's public Go SDK. |
| Providers | Codex and Claude. The user explicitly confirmed that “Cloud” meant Claude. |
| Execution location | First release supports sessions running on the user's computer only. Remote AO Cloud sessions are outside this release. |
| Account UI | Account catalogue and add/login/sign-out/remove controls belong inside AO and are included in the agreed scope. |
| Account-manager location | A separate account manager panel in Settings. Keep the existing Subscription panel for now; remove it in a later change once the new manager is stable. |
| Existing saved accounts | Users sign in again through the new manager. No existing-account import or automatic credential migration in this release. |
| Older sessions | Sessions created before managed-account setup stay on their existing native connection. Switching, removal-driven reassignment and login recovery apply only to sessions created through the new manager; no older-session conversion in this release. |
| Before account-manager setup | Preserve existing normal Codex/Claude login behavior until the user adds/selects managed accounts. This is not fallback for an unavailable assigned account. |
| Primary account | New sessions use the primary unless another account is explicitly selected. |
| Primary per provider | Codex and Claude each have their own primary account. New sessions use the primary matching their provider. |
| First added account | The first successfully added account for each provider automatically becomes its primary. Adding further accounts does not change the primary. |
| Changing primary | Changing the primary by itself affects only new sessions. Account removal/sign-out has the separate reassignment rule below. |
| Removing/signing out a non-primary | Automatically move affected sessions to the matching provider's primary, with a user-visible message as part of removal/sign-out. |
| Removing/signing out the primary | If other usable accounts remain for that provider, require the user to choose a replacement primary before permitting removal/sign-out. Move affected sessions to that primary. |
| Removing/signing out the last account | Allow it with clear information that no account is signed in and login is required. Managed sessions remain present but cannot make model requests; do not fall back to a device-global login. |
| Login after the last account is removed | The next successful login becomes that provider's primary and restores routes for its managed sessions waiting for an account. This restores routing; automatically retrying interrupted work is not authorized. |
| Restart expectation | Aim to change/recover routes without restarting proxy-connected agents. Live Codex GPT-5.5 Low text continuation and recovery after sign-out/re-login passed without a restart; real Claude and broader continuation states remain unverified. Older native-session conversion is excluded by Q09. |
| Busy sessions | Show which affected sessions are using the account and ask the user to wait until they are idle. Do not interrupt their work or complete the switch/sign-out/removal while they are busy. |
| Sign out versus remove | Sign out clears saved login credentials and keeps the account listed as Signed out for later login. Remove also deletes its entry from the list. Neither signed-out nor removed accounts can serve requests or act as an eligible primary. |
| Manual selection | Users can choose an account when creating a session and manually change an individual session's account. |
| Account pinning | An explicit account selection means that session's requests go through that account. Choosing and pinning are the same behavior for this scope. |
| Related AI work | Session-owned AI work uses the session's selected account when the provider matches, rather than the current primary. A separate new AO session still follows the new-session default unless explicitly assigned. |
| Size | Keep authored production integration code small, targeting roughly 3,000 effective LOC. Estimates are not verified counts. |
| Test ratio | User requires a minimum production-to-test ratio, interpreted as 1:4: at least four effective authored test lines for every effective authored production line added by this feature. Track both separately, excluding generated code, upstream dependencies, comments, blank lines and fixture/snapshot data. |
| Current stage | Research, product clarification, implementation and local regression verification completed. Ready for the final architecture/failure review after the user's model switch; live Codex text continuation/recovery passed with two accounts; real Claude and broader conversation states remain unverified. |

## Repository requirements and technical ownership

These follow the repository contract or verified architecture; they are not preferences to ask the user to decide:

- AO services/database own primary preferences and session assignments. CLIProxyAPI owns provider credentials, exchange and refresh.
- New proxy state belongs under AO's resolved data directory, normally `~/.ao`; respect `AO_DATA_DIR`.
- Inference, private control and OAuth callback listeners are loopback-only. The existing daemon's listener/auth rules remain intact.
- A session has a stable private connection ticket; the helper maps it to an exact account. Tickets are not provider login credentials.
- Set connection settings for each AO-launched native process instead of switching the device-global Codex login for every session.
- The helper needs a stable endpoint and an acknowledged route copy because native processes can outlive an AO daemon restart.
- Account identity must have a stable AO reference; upstream filenames/email/plan details are insufficient as the only identity.
- Keep the frontend/CLI thin and regenerate API/sqlc artifacts through their documented sources.

## Questions to ask, in order

### Q01 — No accounts configured

**Status: answered.**

**User decision:** Choose the recommended option: preserve existing normal-login behavior until the user adds/selects managed accounts.

If a user has not added any accounts to the new account manager, should AO still let them create sessions using their existing normal Codex/Claude login, or require them to add an account first?

- **Recommended:** Keep existing normal-login behavior until they add/select proxy accounts.
- Alternative: Require account-manager setup for new Codex/Claude sessions.

Why it matters: determines whether the feature is optional, the no-account experience, and how existing users adopt it. A failed account already selected for a session is a separate case; this choice must not silently reroute that session.

### Q02 — Local and remote sessions

**Status: answered.**

**User decision:** Computer only. Remote AO Cloud sessions are outside the first release.

Should the first release cover sessions running on the user's own computer, or also AO Cloud sessions running on remote machines?

- **Recommended:** Local sessions first.
- Alternative: Include remote AO Cloud sessions in this release.

Why it matters: the proposed helper is on the user's computer. Remote sessions require a separately designed reachable proxy/credential arrangement and are now explicitly excluded from this release. This question is about where a session runs, not Claude accounts.

### Q03 — Accounts already saved today

**Status: answered.**

**User decision:** Sign in again. Provide a separate account manager panel in Settings. Keep the Subscription panel for now; retire it later once the new manager is stable. Q01's existing normal-login behavior remains available before managed-account setup.

For accounts already saved in AO's current Codex account screen, should users sign in again to add them to the new manager, or should AO offer to bring those accounts across?

- **Recommended for minimum scope:** Sign in through the new manager; retain existing native behavior according to Q01.
- Alternative: Offer an explicit existing-account import.

Why it matters: AO's existing native credential catalogue and CLIProxyAPI's credential store are different. Import requires conversion/ownership rules and adds scope; it must not become an unrequested automatic credential migration. Claude's existing native login needs separate handling.

### Q04 — Primary for each provider

**Status: answered.**

**User decision:** Separate primaries: one Codex primary and one Claude primary.

Should Codex and Claude each have their own primary account?

- **Recommended:** A primary Codex account and a primary Claude account.
- Alternative: One overall primary; require an explicit matching account for the other provider.

Why it matters: a Codex account cannot automatically supply a Claude session. Separate primaries are now confirmed.

### Q05 — First added account

**Status: answered.**

**User decision:** Yes: the first successfully added account for each provider becomes primary automatically. Adding further accounts does not change it.

When the first account is added for a provider, should it automatically become primary, or should the user choose primary explicitly?

- **Recommended:** First successfully added account becomes primary; adding further accounts does not change it.
- Alternative: Always require explicit primary selection.

Why it matters: controls onboarding and default behavior for subsequent sessions. Adding another account does not change existing assignments. The later Q06/Q07 answer adds recovery for sessions left without an account after the last account is removed.

### Q06 — Removing an account used by sessions

**Status: answered; user supplied a third option.**

**User decision:** Automatically route affected managed sessions to the provider primary and show a message while removing/signing out. If this is the last account, allow removal with clear login-required information; keep managed sessions but stop their model requests until login. The next successful login becomes primary and restores their routes. Q08 requires waiting for affected sessions to be idle; Q09 excludes older native sessions from conversion and reassignment.

If sessions are still assigned to an account, should users be able to sign it out/remove it, or must they move/close those sessions first?

- Earlier option: Allow removal after showing affected sessions; require manual reassignment or reauthentication.
- Earlier recommended option: Require reassignment or ending those sessions first.
- **Chosen:** Automatically move affected sessions to primary with a message, except when no account remains.

Why it matters: explicitly authorizes reassignment caused by removal/sign-out, without authorizing fallback on quota/auth errors or reassignment merely because primary changed. Upstream removal can affect live provider connections, so implementation must test or arrange a safe removal boundary.

### Q07 — Removing the primary

**Status: answered together with Q06.**

**User decision:** Require selection of a replacement primary before logout/removal when other usable accounts remain for the provider. If this is the last account, show the no-account/login-required consequence and permit removal. The next successful login becomes primary and restores managed sessions waiting for an account.

If the primary is removed or signed out while other accounts remain, should AO ask the user to choose another primary, or automatically choose one?

- Earlier recommendation: Clear the primary and require an explicit choice for new proxy sessions.
- Earlier alternative: Automatically choose an eligible remaining account for future sessions.
- **Chosen:** Require a replacement primary before removal/sign-out unless no other account remains.

Why it matters: settles future-session defaults and the destination for sessions using the removed account. Before initial manager setup, Q01 preserves normal login. After deliberate removal of all managed accounts, managed sessions must show login required rather than silently use that native login.

### Q08 — Account changes and removal during work

**Status: answered.**

**User decision:** Show the user that affected sessions are using the account and ask them to wait until those sessions are idle. Do not interrupt the work. Keep the account and its credentials usable until the operation can complete safely.

**Implementation default for simplicity:** Gate completion on idle state and recheck at execution. This answer does not require a background queue that automatically performs removal later. Define idle using the agent's turn lifecycle, not a brief gap between requests; prevent a new turn from racing with route reassignment/credential removal.

If a session is working when its account is switched or removed, should AO finish the current turn before completing the change/removal, or stop the current turn and change immediately?

- Earlier recommendation: Finish current work first; show the change/removal as pending, then complete it at the idle turn boundary.
- Earlier alternative: Stop current work and apply the change/removal immediately.
- **Chosen:** Inform the user and require waiting until affected sessions are idle; do not interrupt them.

Why it matters: removal may affect several sessions, and credentials cannot be deleted immediately while also promising their current turns can finish. A pause between model requests is not necessarily an idle turn. A queued-action UI is not required by the user's answer.

### Q09 — Sessions created before proxy setup

**Status: answered.**

**User decision:** Option 1 only: leave older sessions as they are. Account switching applies only to sessions created through the new manager. No conversion/restart of older native sessions in this release.

Should manual account switching also cover older sessions started with the normal device login, even if converting them requires restarting/resuming Codex, or only sessions already created through the new account manager?

- **Recommended for minimum scope:** First support manual switching for sessions already connected to the helper.
- Alternative: Also support an explicit conversion/resume of older native sessions.

Why it matters: an existing native process has neither the helper endpoint nor a session ticket. Updating the helper's lookup cannot redirect that process. Primary changes must not trigger conversion automatically.

### Q10 — Related model calls

**Status: answered.**

**User decision:** Option 1 only: related AI work uses the session's selected account when the provider matches.

When AO performs AI work for a session, such as reviewing its changes, should it use that session's selected account when the provider matches, or the current primary?

- **Recommended:** Session-owned work uses the session account; unrelated work uses its own chosen/default account.
- Alternative: Separate reviewer calls use the provider primary.

Why it matters: determines which account receives associated usage. Ordinary prompts, compaction, titles and native child processes belonging to the same session should obey its assignment. A newly created AO session follows the agreed new-session default unless an account is explicitly supplied.

### Q11 — Sign out versus remove in the account list

**Status: answered. Added during the remaining product-gap review.**

**User decision:** Option 1: keep the account listed as Signed out after sign-out; Remove deletes the entry from the list.

When a user signs out, should the account remain in the list marked signed out so they can sign in again, or disappear from the list like a removed account?

- **Recommended:** Sign out clears the saved login and keeps a signed-out entry; Remove also removes the entry from the list.
- Alternative: Both sign out and remove clear the saved login and remove the entry.

Why it matters: the research proposes retaining an AO account row for sign-out, but the user has not yet confirmed that visible behavior. Signed-out entries do not count as usable replacement primaries and cannot serve requests. Both actions obey the confirmed idle, replacement-primary and reassignment rules. This is local sign-out; provider-wide grant revocation has not been verified upstream.

## Settled behavior we should not reopen

- Changing primary by itself never changes existing session assignments or prompts for bulk migration. Explicit removal/sign-out moves the affected sessions as confirmed in Q06/Q07.
- The agreed account catalogue/management screen is included, not a separate feature purchase or later add-on.
- The new manager lives in a separate Settings panel. Existing-account import and immediate Subscription-panel removal are outside this release.
- Older native sessions remain unchanged; their conversion into managed sessions is outside this release.
- All accounts share one helper for approach 1.
- Manual session account selection stays in scope.
- Account changes/removal must wait for affected sessions to be idle and must not interrupt active work.
- A selected account is not a suggestion to freely use other accounts. Quota/auth/model errors do not trigger automatic rotation. Explicit removal/sign-out and recovery after the last account was removed use the confirmed reassignment rules.
- Additional dashboards, quotas/analytics products, routing languages and automatic account pools are outside the current request.

## Technical checks to resolve through research and a prototype

Do not ask the user to decide implementation details that the code/tests should establish:

1. Standard Codex and Claude clients accept the helper settings for both Chat and terminal sessions, including credential precedence and model discovery.
2. Exact account selection survives retries, SDK startup, unchanged configuration reload and credential refresh. The ten existing fake-account request checks cover only part of this.
3. Manual changes, removal-driven reassignment and login after a no-account period preserve a Codex/Claude conversation, or establish the minimum reconnect/resume needed. A mapping update does not prove the native agent will recover after an auth error or that upstream continuation state is portable between accounts.
4. OAuth completes through AO-owned loopback callbacks; login/relogin maps to the intended stable account.
5. Sign-out/delete behavior matches Q06/Q07 and does not unexpectedly interrupt another account's requests.
6. Helper adoption/recovery preserves endpoint, tickets and route revisions across daemon replacement without treating unknown probes as dead processes.
7. Existing model calls and all applicable spawn/restore/interface-transition paths retain the intended account.
8. Packaged helper works on supported OS/architectures and respects AO data paths, generated contracts and CI requirements.

All eleven product questions above are answered. The [implementation plan](implementation-plan.md) translates them into components and acceptance checks. Live compatibility checks remain gates before claiming the integration works or promising no agent restart. Refine code/time estimates after those checks.
