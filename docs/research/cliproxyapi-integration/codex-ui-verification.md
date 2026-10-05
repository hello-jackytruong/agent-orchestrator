# Codex account UI verification — 3–4 October 2026

Only account-manager/session-routing changes. Existing Electron app, existing Chrome accounts, GPT-5.5 Low for every test inference. No Claude operations. Prior successful live evidence is reused; API-only evidence does not certify the native action path. Statuses distinguish previously verified, newly verified, failed and blocked. Current checkpoint: **62 verified, one partial, one not run live**. Later execution notes supersede earlier checkpoints.

Baseline: 38 durable session records. Online backup under `~/.ao/dev/data/backups/before-ui-account-suite-20261003T072006Z.db`. Codex A begins signed out; Claude state is preserved.

| ID | Case | Status / evidence |
|---|---|---|
| 1 | First login becomes primary | Previously verified — Native OAuth, 2 Oct; setup may require login again |
| 2 | Second login preserves primary | Previously verified — Native OAuth, 2 Oct; setup may require login again |
| 3 | Duplicate account login | Passed — completed native Add/OAuth for already-signed-in B; success message, exactly two Codex entries, same IDs/default/routes |
| 4 | Cancel login | Passed — native Cancel removed pending controls; catalogue unchanged |
| 5 | Close browser before login completion | Passed — closed chooser before authorization; AO retained A only and remained pending |
| 6 | Reopen sign-in page | Passed — Open sign-in reopened the same pending login chooser |
| 7 | Close/reopen settings during login | Passed — closed/reopened Settings; pending login controls retained |
| 8 | Cancelled callback cannot add account | Passed — completed cancelled browser authorization; callback refused; A only remained |
| 9 | Wrong identity during sign-in again | Passed — real B Sign in again deliberately selected A in Chrome; Electron reported sign-in failed, B stayed signed out, A/default/seven routes retained; a fresh B authorization restored the same B ID |
| 10 | Catalogue labels/counts | Previously verified — Native catalogue and API evidence, 2/3 Oct |
| 11 | Make another account primary | Previously verified — Native primary change, 2 Oct |
| 12 | Existing routes survive primary change | Previously verified — 2 Oct existing terminal stayed on A |
| 13 | Change primary during active work | Passed — native primary B to A while scratch-9 Chat active; both B assignments and all A assignments retained |
| 14 | Signed-out account eligibility | Passed — signed-out B has no Use as default action; both existing-session and new-task selectors exclude B; new-task still shows Codex / GPT-5.5 / Low |
| 15 | Reopen settings preserves primary | Passed — closed/reopened native Settings after B to A primary change; A retained primary and assignments unchanged |
| 16 | Spawn on primary | Previously verified — 2 Oct terminal A/B inference |
| 17 | Explicit A while B primary | Passed — native Chat creation explicitly chose A while B primary; GPT-5.5 Low replied AO_UI_A_CHAT_READY; stored route A |
| 18 | Explicit B while A primary | Passed — native creation explicitly selected B while A primary; scratch-8 replied AO_UI_B_READY on GPT-5.5 Low |
| 19 | Chat and Terminal creation variants | Passed — native explicit-account creation in both Terminal and Chat produced real GPT-5.5 Low replies |
| 20 | Prepared task account selection | Passed — native Scratch creation selected B while A primary; change log shows prepared record at 07:49:29, promotion at 07:50:21, B inference at 07:50:33 |
| 21 | Primary changes while creation form open | Passed after fix — real form showed B, a concurrent AO API client made A primary before submit, scratch-11 received A. Before fix, reverse race created scratch-10 on stale A; default now leaves primary resolution to daemon. Explicit-choice behavior retains its regression coverage |
| 22 | Selected account removed while creation form open | Passed — Codex form explicitly selected B, a concurrent AO API client removed B, next real catalogue poll excluded it and disabled Start task; selecting primary re-enabled Start task; no session spawned |
| 23 | No-account creation UI | Passed — native form requires login and disables Start task |
| 24 | Idle switch A to B | Passed — native idle Terminal switch A to B; selected label and saved route agree |
| 25 | Idle switch B to A | Passed — native reverse Terminal switch B to A showed Session account changed |
| 26 | Chat and Terminal switch variants | Passed — native idle Chat A to B showed Session account changed and retained conversation; Terminal both directions previously verified |
| 27 | History and native runtime preservation | Passed — native Chat continuation recalled prior marker after A to B; runtime/conversation/controller identities unchanged; Terminal identities previously verified |
| 28 | Other session route unchanged | Passed for B sign-out — the five existing A assignments stayed unchanged |
| 29 | Counts refresh after switch | Passed — native counts reflected manual switch A: five / B: one as well as sign-out reroute |
| 30 | Reopen session retains route | Passed — signed-in scratch-9 pinned B through the actual Chat selector; reopening scratch-8 then scratch-9 retained B and all prior Chat history |
| 31 | Explicit route survives primary change | Passed — explicitly assigned B Terminal retained B when primary changed from A to B; A sessions retained A |
| 32 | Chat/Terminal handoff keeps account | Passed — actual signed-in scratch-9 Chat → Terminal → Chat retained B; Low remained selected in Chat; bold » prompt compatibility repaired after reproducing the old detector failure |
| 33 | Busy switch refusal | Passed — native Chat switch B to A refused while Working; idle warning and B route retained |
| 34 | Busy sign-out refusal | Passed — native Sign out refused during GPT-5.5 Low Terminal work; B remained signed in |
| 35 | Busy removal refusal | Passed — native Remove refused during same active Terminal work; B remained present |
| 36 | Idle retry succeeds | Passed — busy B removal refused; once Chat replied AO_UI_CHAT_IDLE, native retry removed B and rerouted both sessions |
| 37 | One busy session protects entire account | Passed — native B removal refused with scratch-9 active and scratch-8 idle; both routes and account retained |
| 38 | Unsent terminal draft protected | Passed — unsent draft blocked native account switch with idle warning; A retained; draft cleared |
| 39 | Approval wait protected | Passed — actual GPT-5.5 Low request waited for user approval; different-account switch was refused with pending decision still visible and A retained; harmless command then denied |
| 40 | Active Codex reviewer protects account | Not run live — disposable workers have no PR, and AO reviewer prompts require publishing a GitHub review. This conflicts with do-not-publish scope. Real reviewer admission/activity guards are validated by the sequential backend race suite; no live pass claimed |
| 41 | Send/change race | Passed for observed switch-first ordering — actual concurrent UI Send and account select both completed; B mutation won admission, request completed AO_ROUTE_RACE_COMPLETE, B remained selected. Automated tests cover the other fencing orderings |
| 42 | Separate warning footer idle sign-out | Previously verified — Native warning-footer fix sign-out, 3 Oct |
| 43 | Cancel confirmation | Passed — native Cancel dismissed both primary confirmation panels without mutation |
| 44 | Nonprimary sign-out and reroute | Passed — native B sign-out moved scratch-8 to A and retained Signed out row |
| 45 | Nonprimary remove and reroute | Passed — native signed-in nonprimary B removal rerouted Chat scratch-9 and Terminal scratch-8 to A; B catalogue entry removed |
| 46 | Primary sign-out requires replacement | Passed — native primary sign-out requires a replacement; Confirm disabled initially |
| 47 | Primary remove requires replacement | Passed — made B primary with one disposable Chat assignment; Remove required replacement A and Confirm was initially disabled; actual confirmation deleted B and rerouted the preserved conversation to A |
| 48 | Consequence warning | Passed — native confirmation explains reassignment and busy-session wait |
| 49 | Remove signed-out account | Passed — native signed-out B removal removed only its catalogue entry; sessions preserved |
| 50 | Chat and Terminal removal variants | Passed — native account removal affected both idle Chat and Terminal sessions; both preserved and rerouted to primary A |
| 51 | Last sign-out retains catalogue | Previously verified — Native last sign-out, 3 Oct |
| 52 | Last removal preserves sessions | Passed — native last A removal emptied Codex catalogue and showed login-required notice; all 38 baseline records and Chat identities preserved; stored managed Chat route has no account and loginRequired true |
| 53 | No ambient-login fallback | Previously verified — 2 Oct last-account request refused with no model reply |
| 54 | Relogin becomes primary | Previously verified — 2 Oct re-login |
| 55 | Waiting routes restore | Previously verified — 2 Oct all waiting routes restored |
| 56 | Conversation continues without restart | Previously verified — 2 Oct same Chat recalled original phrase |
| 57 | Second login does not steal restored routes | Previously verified — 2 Oct second login preserved primary/routes |
| 58 | Repeated action does not duplicate | Passed for duplicate sign-in — repeated authorization did not duplicate catalogue; earlier double-click Add showed one pending attempt |
| 59 | Close settings during change | Passed — held delivery of a real successful PUT primary response, closed Settings, delivered response, reopened Account Manager; changed primary and all existing pins retained. Controlled response timing, not fake account data |
| 60 | Daemon restart during login | Partial — pending real UI OAuth attempt invalidated by daemon restart; old authorization could not reach its closed relay and registered no account; redelivery after a fresh listener opened was blocked by Chrome ERR_BLOCKED_BY_CLIENT, so live stale-state rejection is not certified. Fresh normal B sign-in succeeded |
| 61 | Daemon restart preserves routes/default | Passed — actual daemon restart with two signed-in Codex accounts retained exact catalogue IDs, A primary, eight A routes/one B route and all 42 native conversation identities; recreated Chat controller generations changed as expected |
| 62 | Helper interruption/recovery | Passed — stopped exact owned helper PID 76627; real B Chat request automatically launched replacement PID 6568 on same loopback port/private identity, recalled AO_UI_A_CHAT_READY AFTER_HELPER_RESTART on GPT-5.5 Low, retained B and all catalogue/default/route facts |
| 63 | Connection interruption/retry | Passed — existing Electron renderer temporarily taken offline; Account Manager displayed Failed to fetch while retaining cached catalogue; online recovery/refetch cleared the alert; saved catalogue/defaults unchanged |
| 64 | Account error never selects alternate | Passed with controlled missing-login fault — private helper API deleted only B local saved login; B Chat retried account_unavailable 503 and produced no alternate-account reply while A remained signed in. Normal browser restoration is pending; no real quota exhaustion claimed |

## New execution notes

- Case 23: native new-task form displayed “Please sign in again in Account Manager”; Start task disabled. GPT-5.5 Low already selected. No session created.

- A re-login completed and restored five routes. Chrome displayed a callback error page after authorization, but the native catalogue and safe daemon view confirmed success. Initial wrong-identity attempt ended with a failed-login message and retained A signed out; exact failure reason is not yet certified.

- Normal B login completed through the native Add account/browser authorization path. Safe daemon view shows A primary with its original five routes, B signed in with zero routes, and unchanged Claude state. Native post-login rendering verification paused when the Mac locked. All 38 session records and baseline lifecycle/conversation/controller fields are unchanged.
- The Mac locked during initial login verification; after the user unlocked it, native catalogue verification resumed. Cases left pending at that point were not counted as passed.

- New native Terminal `scratch-8` explicitly selected B while A remained primary. Codex 0.160.0, GPT-5.5 Low, returned `AO_UI_B_READY`. No commands or file changes were requested.
- Native non-primary B sign-out rerouted `scratch-8` to A, retained B as signed out and refreshed catalogue counts to A: six / B: zero. Native removal of that signed-out entry preserved the session. B was added again using its existing Chrome sign-in; its new catalogue ID has zero assignments and A remains primary.
- A read-only comparison after these operations confirms all 38 baseline records, lifecycle identities and conversation/controller fields unchanged. There are now 39 records including the new dummy. The dummy's native runtime/conversation identities also survived the account operations. Claude facts remain unchanged.
- Native session-account dropdown actions are currently uncertified: accessibility input intermittently loses the window or opens the terminal context menu. ScreenCaptureKit also intermittently reports capture failure. This is an automation limitation, not sufficient evidence of a routing defect. User assistance to foreground AO/dismiss the menu has been requested; no API mutation is substituted for a native pass.

## Earlier checkpoint before the continuation

27 cases have verified evidence (13 reused, 14 newly exercised); six are partial and 31 remain pending. No pending case is certified from implementation or automated tests alone. Native input remains unreliable after window activation: Settings buttons and keyboard focus can remain unchanged, and session-select input can open the terminal context menu instead. The user has been asked to foreground the existing AO window and dismiss that menu. Remaining UI cases await reliable native control; no second app/browser renderer is substituted.

At this checkpoint both Codex accounts are signed in, A is primary, all six managed routes use A, Claude is unchanged, no account recovery journal is pending, and all original 38 session records retain their baseline lifecycle/conversation fields. The new GPT-5.5 Low dummy `scratch-8` remains available for further tests. No new production changes or heavy suite reruns were made in this UI pass.

## Continuation after user requested “continue”

Native Terminal account selection worked for A → B and B → A. An unsent disposable draft prevented reassignment and retained A; after clearing the draft, the same native action succeeded and selected B. Native catalogue counts reflected A: five / B: one. The draft was never submitted.

The same GPT-5.5 Low dummy then ran one bounded `sleep 120` request. The native window showed Working. B sign-out and B removal were both refused with the idle-state warning; B remained signed in and assigned. An attempted native busy switch was interrupted before submission could be certified, so case 33 remains pending. B was subsequently made primary while all existing assignments stayed unchanged. The exact working/idle boundary at that primary click lacks sufficient timing evidence; case 13 remains partial.

Prepared creation is supported by the real sequence: `scratch-8` has a session-created change at 07:49:29 UTC, promotion/update at 07:50:21 and work shortly afterwards. The native form explicitly selected B while A was primary. Frontend task preparation occurs before submission and sends the prepared token to promotion. The selected B route and model reply were independently verified.

Native input/capture degraded again. A disposable secondary shell tab was opened during task navigation; no commands were typed into it. A normal renderer Reload was requested from Electron's View menu without stopping the daemon or agent runtimes, but completion cannot be certified from the inconsistent observations. A screenshot returned an older Account Manager view while AX showed a task form, and a subsequent combined capture was blank. A reset of the computer-control connection did not consistently restore input. This is not enough evidence to label an AO integration defect. The user has been asked to leave the existing Electron window in front and available for testing.

At this continuation checkpoint: 35 verified cases (13 reused, 22 newly exercised), seven partial, 22 pending. No API write has been used to certify a native UI test. Further native checks require reliable desktop control.

## Preserved state at the end of this continuation

Computer control again reported an active-app change before primary restoration could be submitted natively. A was restored as primary through the existing daemon API strictly as test cleanup, not UI evidence. Both Codex accounts are signed in: A has its original five assignments and B has only the new disposable `scratch-8`. Claude is unchanged and no recovery journal is pending. The General preference remains Terminal; the attempted native Chat selection did not persist.

The final read-only comparison confirms all 38 original session records and their runtime/conversation/controller/termination fields match the baseline. There are 39 current records including the new dummy. Its runtime launch and native conversation IDs are unchanged across manual account switching and busy refusal tests. No daemon, helper or native agent restart was performed during this continuation. No production defect is established by the automation failures; no new production code or test-suite reruns were added.

The requested complete native UI pass is **not complete**: 35 verified, seven partial and 22 pending. Native Chat variants, duplicate/wrong-identity login certification, selected-account changes during open creation forms, busy/reviewer/approval variants and controlled process/network recovery remain unverified where the matrix says so. They must not be labelled passed from source coverage or the earlier API-only run.

## Latest native continuation — 3 October

The current matrix supersedes the earlier checkpoint counts: **48 verified (13 reused, 35 exercised), two partial and 14 pending**. No new production defect was established in this continuation, and no heavy suites were rerun.

- Duplicate native Add/OAuth for signed-in B retained its catalogue identity and assignment, while A remained primary.
- Changed the General creation preference to Chat, made B primary, explicitly selected A in the native Scratch task form, and created `scratch-9`. GPT-5.5 Low returned `AO_UI_A_CHAT_READY`. The durable session mode is Chat and its selected route was A.
- Switched the idle Chat to B natively. A subsequent GPT-5.5 Low request returned `AO_UI_A_CHAT_READY CONTINUED`, preserving the earlier conversation. The runtime launch, agent conversation, provider conversation, controller and termination fields matched the post-creation baseline.
- Ran only one bounded `sleep 120` command in that disposable Chat. Its native Working state rejected B-to-A reassignment. Changed primary B to A during the active turn: existing assignments remained unchanged. B removal was refused while `scratch-9` was active and `scratch-8` idle; both B assignments and the account stayed intact. Closing and reopening Settings retained A as primary.
- After the Chat returned `AO_UI_CHAT_IDLE`, native B removal succeeded and rerouted both its Chat and Terminal sessions to A. The catalogue showed A with seven assignments.
- Native removal of the last Codex account A succeeded and displayed the no-account sign-in notice. The stored Chat route remained managed with an empty account and `loginRequired: true`. A read-only comparison confirmed all 38 original records and their lifecycle/conversation/controller fields unchanged; there are 40 records including the two disposable tests. The Chat's runtime identities also remained unchanged. Claude was untouched.
- Restored the original General preference to Terminal through the native menu; a safe settings read confirmed `defaultSessionMode: tui`. No Electron, daemon, helper or agent restart was performed.

Recovery is **blocked by browser permission**, not certified as complete. The Chrome tool first reported denied access to `auth.openai.com`; after the user requested autonomous continuation, the same authorized tool explicitly reported a **saved user permission setting** blocking that domain. No alternate browser surface or credential workaround was used. A fresh native Add-account attempt opened a recovery sign-in page, but the current durable catalogue has no Codex account. The user must enable the domain permission before automated OAuth recovery can proceed. Existing sessions remain preserved and waiting for sign-in; do not report restored accounts until the catalogue and routes actually confirm it.

Remaining cases are 9, 21, 22, 30, 32, 39, 40, 41 and 59–64; cases 14 and 47 remain partial. Account-dependent checks and cleanup must resume after the saved browser permission allows recovery. The automation capture/focus issues described above remain intermittent; the successful actions here were individually verified and were not replaced with API mutations.

## Safe completion pass after the user's skip instruction

The user explicitly instructed the agent to skip suspicious actions and continue independently. All remaining cases were assessed: **48 verified, two partial, 14 skipped**. The matrix now records individual skip reasons. Skipping is not a pass; the missing signed-in-account variants and native recovery scenarios remain coverage gaps.

The existing dev daemon still reports no Codex account. The saved browser restriction was not bypassed, and credentials were not restored from backups or imported through another surface. Native navigation then reported that the Mac was locked. Further desktop checks were skipped without repeated unlock/permission requests. Live daemon/network disruption against original user data was also skipped; prior automated race, transport, restart and helper-recovery evidence remains distinct from native UI evidence.

A concrete UI defect was observed before the lock: a refused account-switch warning remained visible after subsequent account removal changed the route to login-required. `SessionProviderAccount` now associates notices with the owning session and account, and discards them when that route changes. Four regression tests cover external rerouting, last-account removal, obsolete success notices, and a delayed refusal arriving after account management changed the route. The first three tests failed before the fix; the final component/journey run passed **18 tests**. Typecheck passed. The fixed component was copied to the existing dev checkout only after confirming that its prior source matched the unfixed implementation; Electron was not restarted. Native post-fix rendering is not certified while the desktop is locked.

Effective authored counts are **2,935 production lines and 12,439 test lines (4.24:1)** using the existing count script and exclusions. The fix adds three effective production lines and 54 effective test lines to the previous count. No generated API or database contract changed.

Final validation: frontend typecheck passed; `npm run build` passed through Electron Forge's local darwin-arm64 packaging, including daemon, bundled tmux, browser/ACP runtime preparation and production main/preload/renderer bundles. The component/journey tests used one worker, and build validation followed them sequentially. `git diff --check` passed. No publication, deployment, app launch or process restart occurred. Original 38 session records and their runtime/conversation/controller/termination fields still match the baseline; there are 40 records including the two disposable sessions. All ten live sessions are idle; Chat runtime identities and Claude account facts are unchanged. The default interface is Terminal. Codex still has zero signed-in accounts, so live recovery and account restoration remain blocked by the saved browser permission rather than completed.


## Playwright continuation and controlled recovery — 3 October, 16:03–16:26 UTC

The user explicitly authorised Playwright as well as native computer use. Playwright attached by loopback CDP to the **same dev Electron checkout, profile and database**, after an online SQLite backup to `~/.ao/dev/data/backups/before-remaining-ui-restart-20261003.db`. No browser-only AO renderer, scratch profile or copied credentials was substituted. An initial native screenshot showed the login-required session selector without the obsolete busy warning, providing rendering evidence for that specific stale-notice fix.

The native tool nevertheless reported that the Mac was locked. Electron's hidden renderer paused the animation frame which mounts Settings content. Playwright's test clock briefly allowed opening the real Account Manager and clicking Codex Add account. The approved Chrome tool then rejected access to `auth.openai.com` again because of the saved browser permission. This denial was not bypassed with raw browser commands, another browser, credential restoration or another OAuth surface.

Case 60 gained partial live evidence: a real pending UI login existed before the daemon restart. After restarting through Electron's existing daemon bridge, its status returned `PROVIDER_LOGIN_NOT_FOUND` with “login attempt not found; sign in again”. The saved catalogue was unchanged. Old callback delivery and the complete fresh-login UI retry were not exercised. Electron's hidden-renderer automation remained unreliable after restart; the test clock and temporary debugging environment were removed by a normal app relaunch without debug flags. No listener remains on port 9337.

Case 61 gained partial live evidence: the actual app/daemon restarts preserved all 40 session records, runtime launch IDs, native agent session IDs, provider conversation IDs and termination facts. Only the controller generations of Chat sessions `scratch-5` and `scratch-9` changed, as expected when a daemon recreates controllers. This supersedes the earlier statement that controller generations were unchanged throughout the account-only operations. Codex's managed login-required routes and empty primary persisted; a signed-in default/route restart is still unverified.

Case 62 gained partial service-boundary evidence: after verifying the exact owned helper command and listener, PID 74057 was terminated. Starting a temporary Codex login through AO's service automatically launched replacement PID 76627 on the same loopback port 63759. The new attempt reached waiting, was cancelled (204), and changed no saved accounts/defaults. Both dummy sessions remained managed, login-required and retained their runtime/conversation IDs. This was controlled fault setup through the daemon API, **not** authenticated UI continuation or a successful browser login.

No additional inference or Claude operation was performed. The existing app remains open on its normal launch configuration, with Terminal as the default interface. Codex still has zero signed-in accounts; Claude facts are unchanged. Account restoration and the signed-in variants remain blocked by the saved Chrome permission. Cases 9, 21, 22, 30, 32, 39, 40, 41, 59, 63 and 64 are not certified; cases 14, 47 and 60–62 remain partial. The 48 earlier verified cases were not repeated for a new pass claim. No production change or heavy test rerun was made in this continuation.

A separate `/notifications` 500 was observed during startup: the database lacks `notifications.source_key`. Read-only inspection confirmed that the backup taken before this UI suite also lacks it, and its schema belongs to migration 170, outside the account-manager migration 173. No unrelated schema repair or direct database write was made. This is an existing development-database compatibility gap, not evidence of an account-routing failure.


## Unlocked desktop retry — 3 October, 16:35 UTC onward

The native tool could access the Mac again. The existing Electron window initially appeared blank, so the same dev checkout/profile/data was backed up and relaunched with temporary loopback debugging and Chromium background-throttling test flags. Playwright then operated the real application without installing a test clock. Native capture/accessibility remained sparse; functional evidence comes from the actual Electron renderer controls and DOM, with safe daemon reads used to check saved facts.

Case 63 passed: only the Electron browser context was temporarily set offline. Account Manager showed a real “Failed to fetch” alert while retaining its cached catalogue. Restoring online access and revisiting Account Manager caused a successful refetch, removed the alert and preserved saved accounts/defaults. No operating-system network setting was changed.

Case 60 gained UI evidence: a pending Codex Add account attempt was interrupted by a restart through Electron's existing daemon bridge. After returning to Account Manager, the UI displayed “Sign-in failed. Please sign in again”, removed waiting controls and enabled Add. A fresh UI attempt reached waiting and was cancelled. Delivery of an authorised old callback remains untested because Chrome still denies access to auth.openai.com.

Case 30 gained partial UI evidence: both disposable sessions were reopened, followed by Terminal-to-Chat navigation. Their login-required routes remained visible, and Chat retained AO_UI_A_CHAT_READY CONTINUED and GPT-5.5 Low. Reopening a signed-in account pin remains unverified.

Chrome's approved browser tool again explicitly rejected auth.openai.com because of a saved user permission. No alternative browser/credential path was used. The user asked how to change it; official OpenAI help confirms allowed/blocked website access is managed in desktop Settings. The denied permission must be changed before account-dependent UI tests and Codex account restoration can proceed. Current matrix: **49 verified, six partial, nine blocked/skipped**.

The user then supplied a screenshot showing Chrome's default browsing permission as “Always allow”, with no auth.openai.com exception visible. A fresh Codex Add account attempt opened the account chooser, but claiming that tab through the approved Chrome tool still returned an explicit saved-permission denial for auth.openai.com. This is a mismatch between the visible settings and the browser tool's permission check; the screenshot does not establish that the tool actually allowed access. The fresh pending attempt was cancelled through Electron's Cancel button, and the no-signed-in-Codex message returned. No browser-control workaround was attempted and no additional test was marked passed.

## Permission-update retries and handoff diagnosis — 3 October, 22:38 IST onward

After the user reported updating permissions and restarting, Chrome connected again to the same extension/profile. Claiming the observed account chooser still returned an explicit saved-permission rejection for auth.openai.com. Another reported permission update produced the same rejection. No native-Chrome, raw CDP, credential-import, alternate-browser or other workaround was attempted. No sign-in-dependent case was certified.

The existing Electron app had been closed during the restart. Its same checkout, dev profile and data were backed up to `~/.ao/dev/data/backups/before-permission-retry-20261003.db` and relaunched for the independent handoff check. The real Switch to chat UI control again refused the disposable scratch-9 transition with the quiescence-unverified message. A saved screenshot (`output/playwright/cliproxy-permission-retry.png`) shows retained history and login-required routing. No model request was submitted; the resumed Terminal footer displayed GPT-5.5 Ultra, so it was not used for inference under the user's Low-only constraint.

A read-only styled-output request to the registered scratch-9 PTY host identified the current prompt as a bold `»`, followed by the dim “Ask Codex to do anything” placeholder and normal model/warnings footer. `InspectTerminalSurface`, `codexPromptFooter` and composer parsing only recognise `›`; the same restriction is present in `origin/main`. This establishes an existing Codex terminal compatibility gap, rather than evidence that account routing lost the conversation. Chat-to-Terminal retained the provider conversation, but return-to-Chat cannot be certified while that idle proof fails. The guard was not bypassed, and original sessions were not modified to resolve it. Case 32 is now partial, not skipped solely due to desktop locking. Current matrix: **49 verified, seven partial, eight blocked/skipped**.

Cleanup relaunched the same dev checkout/profile/data without remote-debugging or background-throttling test flags. The daemon served its API normally. A read-only comparison retained all 38 original records, runtime/native-agent/provider-conversation IDs and termination flags; scratch-5's Chat controller generation changed during the earlier daemon restarts. There remain 40 records including the two disposable sessions. Codex has zero signed-in accounts and no recovery journal pending. No new production change, inference or heavy suite run was made. At the user's subsequent retry request, the old chooser tab was gone; opening a fresh chooser through the approved Chrome tool still produced the same saved-permission rejection.

The user supplied a 22:49 IST settings screenshot confirming an exact `https://auth.openai.com` Browsing rule set to Always allow, in addition to the Always allow default. One fresh approved Chrome-tab retry still returned the saved-permission rejection. The visible per-site rule is correctly configured; the tool's contrary result is an unresolved permission-state mismatch, not evidence that the user failed to enable access. Further requests to set the same rule are not a useful resolution. Sign-in-dependent cases remain blocked, without any bypass or new pass claim.

Read-only permission diagnosis independently opened and read GitHub and the official documentation site in Chrome, including a fresh documentation origin. This rules out a general disconnected-browser or disabled-extension failure. Local `~/.codex/config.toml` now also contains `browser_use.origins."https://auth.openai.com".access = "allow"`, corroborating the screenshot. The rejection remains specific to the sign-in origin and occurs in the tool's pre-access permission check. Stale chat permission state or another inconsistent policy source remains a hypothesis, not a confirmed root cause. Chrome extension site access has not been verified: the browser tool separately prohibits `chrome://extensions` by URL-scheme policy, so no alternate surface was used to inspect that blocked page. Official Google instructions identify the extension's Details → Site access as the independent Chrome-side setting; an existing all-sites permission would need no expansion. Official OpenAI troubleshooting recommends a fresh chat to distinguish chat-specific connection state.

## Permission-restored continuation — 4 October 2026

Chrome access succeeded through the approved browser tool. Same Electron checkout, profile and data resumed, with an online backup at `~/.ao/dev/data/backups/before-final-ui-20261004T060128Z.db`. Two existing Codex identities were signed in; A became primary and restored seven waiting routes without restarting agent sessions. Claude was untouched.

Codex 0.160 uses both `›` and `»` prompts. The terminal adapter now selects the current actual glyph without rewriting drafts or discarding styling. Thirty-four focused compatibility cases preserve draft, active-turn and approval safeguards; the glyph cases failed before the change, and adapter/terminal-ui plus focused manager checks passed sequentially. Actual Terminal → Chat and both signed-in handoff directions now succeed. A separate scratch-8 idle switch refusal is under investigation and is not hidden by these passes.

Cases 9, 14, 30 and 32 are newly complete. Evidence: `output/playwright/cliproxy-final-accounts-restored.png` and `cliproxy-final-signed-out-picker.png`; the latter was recaptured with the real creation form open. The 49 previously verified cases have not been recounted as new tests. Remaining work continues.

The open-form test exposed a stale-default defect: the UI sent its cached primary as an explicit account. One production-line change now sends an account ID only for an explicit choice, letting AO resolve the default at admission. Four default-resolution tests failed before this fix; the complete TaskComposer/NewTaskDialog/ProviderAccountJourney run passed 88 tests with one worker. Two disposable sessions (scratch-10 and scratch-11) were created for before/after evidence.

A temporary read-only diagnostic showed the refused scratch-8 switch originated in conservative draft proof, not in helper fallback. A subsequent retry succeeded on an empty styled composer and switched back to A. Temporary diagnostic source changes have been restored exactly; no guard decision was bypassed.

The controlled B-login deletion was followed by a genuine A reply `AO_A_STILL_AVAILABLE`, demonstrating that a healthy alternate existed while B produced only account-unavailable retries. Primary B removal then required A, preserved the conversation, and rerouted it. B was restored by ordinary Chrome OAuth; the removed-selection and stale-login tests then ran. The final signed-in recovery test has A primary, B separately pinned to scratch-9, and nine total managed routes.

Case 60 has one explicit evidence limit: after the old callback failed against the closed pre-restart relay, Chrome blocked its reload against the fresh listener with `ERR_BLOCKED_BY_CLIENT`. No lower-level browser or callback-code workaround was attempted. The fresh login succeeded normally. Automated stale-state and cancelled-callback tests remain distinct evidence.

## Final live checkpoint — 4 October 2026

**62 of 64 cases verified, one partial (60), one not run live (40).** Of the 15 previously remaining cases, 13 are newly completed; the other two have concrete recorded limits. Previously verified cases were reused rather than rerun for a new pass count. All test inference used GPT-5.5 Low. UI actions used the same real Electron app; concurrent-primary/removal setup, missing-login injection and helper termination are explicitly controlled boundary fixtures. No fake UI catalogue, direct database writes, Claude inference, remote credential revocation, publication or deployment occurred.

Helper recovery completed through the actual Chat Send control: B recalled the conversation’s original A marker after helper replacement. A remained primary, the B pin stayed B, and the helper retained its port/private identity. The signed-in daemon restart also preserved native runtime/conversation identities; Chat controller generations were recreated normally.

Validation: the complete TaskComposer, NewTaskDialog and ProviderAccountJourney files passed 88 tests with one worker. Sequential `go test -race -p 1` passed Codex, terminal UI, session manager (145s) and reviewer suites. The sandbox-only race attempt could not read existing cache artifacts; the authorized cache-access run passed. Effective authored counts: **2,963 production / 12,576 test lines (4.24:1)**, excluding generated artifacts and dependency records as in earlier counts. Additional final build/typecheck and cleanup results follow.

## Cleanup and readiness

Frontend typecheck, backend `go build ./...`, relevant `go vet`, and `git diff --check` passed after the fixes. The same existing checkout/profile/data was relaunched normally; temporary Chromium test flags and loopback debug port 9337 are removed, and the old Playwright connection was closed. Temporary daemon diagnostics were restored exactly. The normal managed daemon serves port 3002.

All 38 original sessions retain their native runtime/agent/provider-conversation identities and termination facts. There are 42 durable sessions, including the two earlier dummies and scratch-10/scratch-11 added only for before/after form-race evidence. Chat controller generations changed normally during daemon recreation. Claude catalogue/default facts are unchanged. Both Codex accounts are signed in; A is primary. The disposable scratch-8/scratch-9 pins were restored to A after the recovery checks, so all nine managed routes now use A and B has zero assignments. No account recovery journal remains pending. Native capture can bind an unrelated app because two running Electron apps share the bundle ID; final normal-launch identification therefore uses the exact checkout process and successful daemon reads, while UI screenshots come from the individually verified AO renderer before cleanup.

**Ready for final review with two explicit live evidence gaps (40 and part of 60); not a claim that all 64 live cases passed.** The additional terminal-compatibility and default-resolution fixes need inclusion in that review. The reviewer gap requires a safe nonpublishing review fixture; the stale redelivery gap requires Chrome to allow that specific old localhost callback. No production credential was copied or exposed, and no permission/security bypass was used.
