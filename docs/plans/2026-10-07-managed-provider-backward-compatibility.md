# Managed provider backward-compatibility plan

## Goal

Move Codex and Claude to Account Manager for all new work without breaking
conversations that were created before Account Manager was introduced.

The change is provider-specific. It does not change providers that Account
Manager does not support.

## Final decisions

- Codex and Claude are managed providers.
- The Harness page checks only Account Manager for those providers.
- A native login does not count as “logged in” once the provider is managed.
- If no managed account exists, Harness shows **Sign in through Account
  Manager**, even when native credentials are still present.
- The first successful managed login becomes that provider’s default account.
- Every new session for a managed provider uses a managed account. It uses the
  provider default unless the user chooses another managed account.
- Sessions created before this change keep using their native account.
- Native credentials are kept internally so those older sessions can continue
  after restart. They are no longer offered as a new-session login path for a
  managed provider.
- Moving an old native session is optional and user-triggered. There is no
  silent migration.
- If all managed accounts are signed out or removed, new managed sessions ask
  the user to sign in again. They never silently fall back to native login.
- Codex and Claude keep separate account lists and separate defaults.
- Providers outside Account Manager keep their current native behavior.

## What the user sees

### Harness

For Codex or Claude:

1. Ask the daemon whether a managed account exists.
2. If one exists, show the provider as connected.
3. If none exists, show **Sign in** and open Account Manager sign-in.
4. Do not show a native login button or use native auth as a fallback.

For an unsupported provider, keep the existing native status and login flow.

### New session

When the user starts a new Codex or Claude session:

1. Check the managed account list.
2. Use the selected managed account, or the provider default.
3. If no managed account is signed in, stop before launch and show the Account
   Manager sign-in action.

The session must never accidentally inherit the machine’s native account.

### Existing native session

An older session keeps its original native route, including after AO restarts.
Its history and normal controls remain available.

The session may show **Move to Account Manager**. If the user chooses it, AO
waits for a safe handoff, starts the same conversation with a managed account,
and keeps the old native route untouched until the handoff succeeds.

### Account changes

Changing a provider default affects new sessions and the next request of
managed sessions according to the existing Account Manager switch rules.
Older native sessions are outside that system and are not changed silently.

Signing out or removing a managed account follows the current transfer and
replacement-default rules. Older native sessions are unaffected.

## Implementation plan

### 1. Define the adoption boundary in the daemon

- Treat the presence of Account Manager state for a managed provider as the
  source of truth for new sessions.
- Reuse the existing provider-account state and empty-primary behavior where
  possible; add a new durable field only if the current state cannot clearly
  distinguish “never adopted” from “adopted but currently signed out.”
- Keep the native authentication records needed by older sessions, but mark the
  managed route on every new session created after adoption.
- Keep Codex and Claude independent.

### 2. Return one clear provider status to the UI

Add or adjust the daemon response used by Harness so it can say:

- managed provider with an available account;
- managed provider with no account and sign-in required;
- unsupported provider using native auth.

The response must not expose credentials or provider-private data.

### 3. Change Harness behavior

- For Codex and Claude, render only the managed status and managed sign-in
  action.
- Keep a small explanation when native credentials exist but no managed account
  exists: “Native login is available, but Account Manager is required for new
  sessions.”
- Remove the native login action from these managed-provider cards.
- Leave other provider cards unchanged.

### 4. Make session creation choose the correct route

- At session creation, resolve the provider’s managed account before spawning.
- Attach the existing managed proxy route/ticket to a managed session.
- Mark pre-feature sessions as native and preserve their current launch path.
- Reject a new managed session with a friendly sign-in-needed error when no
  managed account exists.
- Do not use native credentials as an automatic fallback.

### 5. Add the optional old-session migration action

- Expose whether a session is an older native session.
- Show **Move to Account Manager** only for that case and only when a managed
  account is available.
- Reuse the existing safe handoff/recovery path: wait for an idle boundary,
  create the managed route, resume the same conversation, and report a clear
  success or retry message.
- Keep the old native session if any step fails.

### 6. Keep account changes consistent

- A first managed login sets the provider default.
- Default changes continue to affect managed sessions under the existing
  request-switching rules.
- Account sign-out/removal continues to transfer eligible managed sessions to
  the default account or leave them waiting for a new login when no account
  remains.
- Native sessions are never included in managed-account counts or transfers.

### 7. Update copy and remove conflicting guidance

Replace messages that say “existing sessions keep their account” when they are
describing managed sessions. Use separate wording for:

- managed sessions, which follow the account-switching rules;
- old native sessions, which keep their native account until the user migrates
  them.

## Tests

### Backend

- Managed provider with a managed account reports connected.
- Managed provider with only native credentials reports sign-in required.
- Unsupported provider still reports native login state.
- First managed login becomes the default.
- New session uses the managed default.
- New session can use a selected managed account.
- New managed session is rejected when no managed account exists.
- Old native session keeps its native launch route after restart.
- Managed account changes do not rewrite old native routes.
- Optional migration succeeds at a safe boundary and preserves the old route
  when it fails.
- Sign-out/removal transfers only managed sessions.
- Removing the last managed account blocks new managed sessions without
  affecting old native sessions.
- Codex and Claude state remain independent.

### Frontend

- Harness shows Account Manager sign-in when native login is the only login.
- Harness does not show native login for Codex or Claude.
- Unsupported providers keep their current native controls.
- New-session UI offers only managed accounts for Codex and Claude.
- Old native sessions show the optional migration action.
- User-facing errors contain an action, not internal routing details.

### End-to-end acceptance checks

1. Start with only a native Codex login. Harness asks for Account Manager
   sign-in; an old Codex chat still opens.
2. Add a managed Codex account. It becomes default; the next new chat uses it.
3. Add a second managed Codex account and choose it for one new chat. Other new
   chats use the default.
4. Sign out all managed Codex accounts. New chats ask for Account Manager login;
   the old native chat still works.
5. Repeat the same checks for Claude.
6. Confirm an unsupported provider is unchanged.

## Completion criteria

The work is complete when:

- no new Codex or Claude session starts through native auth;
- old native sessions continue across an AO restart;
- the Harness page gives the same answer as the managed account list;
- no managed-account operation changes an old native session silently;
- the optional migration is safe and reversible on failure;
- the backend, frontend, and end-to-end tests above pass without network calls
  in unit tests.

```mermaid
flowchart TD
    A[Open Harness] --> B{Account Manager supports provider?}
    B -- No --> C[Keep native login flow]
    B -- Yes --> D{Managed account exists?}
    D -- Yes --> E[Show connected]
    D -- No --> F[Show Account Manager sign-in]

    G[Create new session] --> H{Managed account available?}
    H -- Yes --> I[Use selected account or provider default]
    H -- No --> J[Ask user to sign in]

    K[Old native session] --> L[Keep native route]
    L --> M[Optional user action: Move to Account Manager]
    M --> N[Safe handoff, then managed route]
```

