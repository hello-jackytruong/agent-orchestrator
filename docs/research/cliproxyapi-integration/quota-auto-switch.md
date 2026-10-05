# Codex quota auto-switch

The account manager now has an optional Codex-only setting for confirmed usage
limit errors. It is off by default. CLIProxyAPI owns credential execution and
reports a durable quota event; AO decides whether the account is still the
current primary before changing anything.

When enabled, AO chooses the first other signed-in Codex account in catalogue
order. It does not inspect that account's quota before switching. It makes that
account primary and moves every session currently using the old primary.
Sessions on other accounts are left alone. An API call already admitted by the
helper keeps its original account; the next call observes the new route.

The setting can be enabled only when two Codex accounts are signed in. With one
or zero signed-in accounts, Account Manager keeps the setting visible but
disabled and shows an information hint. If an account is later signed out or
removed so that only one remains, AO disables the setting again. A replacement
can therefore be unavailable only after a later account change or a recovery
race; the event remains pending in that case.

The helper stores quota events beside its routing snapshot until AO acknowledges
them. AO acknowledges stale events, reset-expired events, and successfully
handled events. A delayed event carries the Codex primary generation that saw
the failure, so an A→B→A user choice cannot be undone by an old report.

Temporary rate limits, authentication failures, and provider outages do not
create quota events. Existing retry and provider-failure behavior remains
unchanged for those cases.
