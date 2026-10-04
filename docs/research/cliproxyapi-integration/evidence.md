# Evidence and verification

Source snapshot: 1 October 2026. Claims about future AO routes, persistence and process behavior in the approach documents are proposed design. Claims below describe the inspected source or tests actually run.

## Pinned source checkouts

| Repository | Revision | Local reference |
| --- | --- | --- |
| AO, fetched from `upstream/main` and fast-forwarded from a clean working tree | `d46cc4d190a248094f44fcfefa7b84eb05109fd9` | `/Users/adilshaikh/Desktop/reverb` |
| CLIProxyAPI, shallow reference clone | `fd48ea6840f5572deb53aeb5657740937ac9daaa` | `/tmp/ao-cliproxyapi-reference-20261001` |

No product code or existing account data was changed during this research. The reference clone, build cache, dependency cache and SDK probe live under `/tmp`.

## AO source anchors

| Finding | Source |
| --- | --- |
| Go daemon/thin frontend architecture; native Chat hosts survive daemon replacement | [Architecture](https://github.com/Untrivial-ai/agent-orchestrator/blob/d46cc4d190a248094f44fcfefa7b84eb05109fd9/docs/architecture.md#L3) |
| Existing Codex account functionality | [Shipped status](https://github.com/Untrivial-ai/agent-orchestrator/blob/d46cc4d190a248094f44fcfefa7b84eb05109fd9/docs/STATUS.md#L134-L143), [controller](https://github.com/Untrivial-ai/agent-orchestrator/blob/d46cc4d190a248094f44fcfefa7b84eb05109fd9/backend/internal/httpd/controllers/codex_accounts.go#L22) |
| Device-global credential switching does not transactionally reassign live controllers | [Coordinator](https://github.com/Untrivial-ai/agent-orchestrator/blob/d46cc4d190a248094f44fcfefa7b84eb05109fd9/backend/internal/service/agent/codex_account_switch_coordinator.go#L111) |
| Legacy active-pointer/restart state was removed | [Migration 0146](https://github.com/Untrivial-ai/agent-orchestrator/blob/d46cc4d190a248094f44fcfefa7b84eb05109fd9/backend/internal/storage/sqlite/migrations/0146_codex_account_management_simplification.sql#L67) |
| Common native launch environment seams | [Session manager runtime environment](https://github.com/Untrivial-ai/agent-orchestrator/blob/d46cc4d190a248094f44fcfefa7b84eb05109fd9/backend/internal/session_manager/manager.go#L5117), [worker/Chat preparation](https://github.com/Untrivial-ai/agent-orchestrator/blob/d46cc4d190a248094f44fcfefa7b84eb05109fd9/backend/internal/session_manager/manager.go#L5210) |
| Codex Chat app-server argv needs custom-provider configuration propagation | [Driver](https://github.com/Untrivial-ai/agent-orchestrator/blob/d46cc4d190a248094f44fcfefa7b84eb05109fd9/backend/internal/adapters/chatdriver/codexappserver/driver.go#L467) |
| Claude recognizes gateway authentication/model discovery | [Credential provider](https://github.com/Untrivial-ai/agent-orchestrator/blob/d46cc4d190a248094f44fcfefa7b84eb05109fd9/backend/pkg/agentcreds/providers.go#L60), [ACP driver](https://github.com/Untrivial-ai/agent-orchestrator/blob/d46cc4d190a248094f44fcfefa7b84eb05109fd9/backend/internal/adapters/chatdriver/claudeacp/driver.go#L79) |
| Existing Codex device auth preflight requires route-aware treatment | [Session service](https://github.com/Untrivial-ai/agent-orchestrator/blob/d46cc4d190a248094f44fcfefa7b84eb05109fd9/backend/internal/service/session/service.go#L321) |
| Desktop project and standalone creation use different daemon operations | [Task composer](https://github.com/Untrivial-ai/agent-orchestrator/blob/d46cc4d190a248094f44fcfefa7b84eb05109fd9/frontend/src/renderer/components/TaskComposer.tsx#L198) |
| Live host adoption preserves original provider environment | [Host contract](https://github.com/Untrivial-ai/agent-orchestrator/blob/d46cc4d190a248094f44fcfefa7b84eb05109fd9/backend/internal/adapters/chatdriver/persistenthost/host.go#L106), [attach path](https://github.com/Untrivial-ai/agent-orchestrator/blob/d46cc4d190a248094f44fcfefa7b84eb05109fd9/backend/internal/adapters/chatdriver/persistenthost/host.go#L289) |

## Routing and SDK

| Finding | Source |
| --- | --- |
| Current module is `/v8`, Go minimum `1.26.0`; AO declares `1.27.1` | [Upstream go.mod](https://github.com/router-for-me/CLIProxyAPI/blob/fd48ea6840f5572deb53aeb5657740937ac9daaa/go.mod#L1-L3), [AO go.mod](https://github.com/Untrivial-ai/agent-orchestrator/blob/d46cc4d190a248094f44fcfefa7b84eb05109fd9/backend/go.mod#L1-L3) |
| Public config aliases avoid external imports of internal packages | [SDK config](https://github.com/router-for-me/CLIProxyAPI/blob/fd48ea6840f5572deb53aeb5657740937ac9daaa/sdk/config/config.go#L1-L18) |
| Public middleware/router configurator | [SDK options](https://github.com/router-for-me/CLIProxyAPI/blob/fd48ea6840f5572deb53aeb5657740937ac9daaa/sdk/api/options.go#L17-L30) |
| Exported handler auth manager and custom selector interface | [Handler](https://github.com/router-for-me/CLIProxyAPI/blob/fd48ea6840f5572deb53aeb5657740937ac9daaa/sdk/api/handlers/handlers.go#L364), [selector](https://github.com/router-for-me/CLIProxyAPI/blob/fd48ea6840f5572deb53aeb5657740937ac9daaa/sdk/cliproxy/auth/conductor.go#L77-L80) |
| Router configurator runs before listener starts | [Server construction](https://github.com/router-for-me/CLIProxyAPI/blob/fd48ea6840f5572deb53aeb5657740937ac9daaa/internal/api/server.go#L236), [service lifecycle](https://github.com/router-for-me/CLIProxyAPI/blob/fd48ea6840f5572deb53aeb5657740937ac9daaa/sdk/cliproxy/service_lifecycle.go#L126) |
| Default builder initializes applied routing state; changed routing settings replace selector | [Builder](https://github.com/router-for-me/CLIProxyAPI/blob/fd48ea6840f5572deb53aeb5657740937ac9daaa/sdk/cliproxy/builder.go#L249-L266), [config application](https://github.com/router-for-me/CLIProxyAPI/blob/fd48ea6840f5572deb53aeb5657740937ac9daaa/sdk/cliproxy/service_config.go#L215-L220) |
| Middleware request-context pins are not inherited by ordinary handlers | [Responses caller](https://github.com/router-for-me/CLIProxyAPI/blob/fd48ea6840f5572deb53aeb5657740937ac9daaa/sdk/api/handlers/openai/openai_responses_handlers.go#L664), [context construction](https://github.com/router-for-me/CLIProxyAPI/blob/fd48ea6840f5572deb53aeb5657740937ac9daaa/sdk/api/handlers/handlers.go#L481) |
| Gin request headers reach executor options in HTTP and SSE | [Header extraction](https://github.com/router-for-me/CLIProxyAPI/blob/fd48ea6840f5572deb53aeb5657740937ac9daaa/sdk/api/handlers/handlers_context.go#L155-L161), [HTTP options](https://github.com/router-for-me/CLIProxyAPI/blob/fd48ea6840f5572deb53aeb5657740937ac9daaa/sdk/api/handlers/handlers_execution.go#L82-L95), [stream options](https://github.com/router-for-me/CLIProxyAPI/blob/fd48ea6840f5572deb53aeb5657740937ac9daaa/sdk/api/handlers/handlers_stream.go#L341-L354) |
| Exact metadata pin filters candidates; custom selector errors do not fall back | [Legacy selection](https://github.com/router-for-me/CLIProxyAPI/blob/fd48ea6840f5572deb53aeb5657740937ac9daaa/sdk/cliproxy/auth/conductor_selection.go#L1758-L1797), [scheduler filter](https://github.com/router-for-me/CLIProxyAPI/blob/fd48ea6840f5572deb53aeb5657740937ac9daaa/sdk/cliproxy/auth/scheduler.go#L708-L718) |
| Stock affinity is soft and can fail over | [Config comments](https://github.com/router-for-me/CLIProxyAPI/blob/fd48ea6840f5572deb53aeb5657740937ac9daaa/config.example.yaml#L133-L150), [affinity selector](https://github.com/router-for-me/CLIProxyAPI/blob/fd48ea6840f5572deb53aeb5657740937ac9daaa/sdk/cliproxy/auth/selector.go#L909) |
| Responses WebSocket keeps/injects a connection account pin | [WebSocket handler](https://github.com/router-for-me/CLIProxyAPI/blob/fd48ea6840f5572deb53aeb5657740937ac9daaa/sdk/api/handlers/openai/openai_responses_websocket.go#L723-L775) |

## Management and operations

| Operation | Upstream endpoint |
| --- | --- |
| Initiate login | `GET /v8/management/oauth/auth-url?provider=codex` or `claude` |
| Submit callback | `POST /v8/management/oauth/callback` |
| Poll login | `GET /v8/management/oauth/status?state=...` |
| Cancel pending login | `DELETE /v8/management/oauth/session?state=...` |
| List credentials | `GET /v8/management/credentials` |
| Disable/enable | `PATCH /v8/management/credentials/status` |
| Delete local credential | `DELETE /v8/management/credentials?name=...` |
| Refresh | `POST /v8/management/credentials/refresh` |

The [v8 route registry](https://github.com/router-for-me/CLIProxyAPI/blob/fd48ea6840f5572deb53aeb5657740937ac9daaa/internal/api/server_management_v8.go#L43-L54) and [OAuth dispatcher](https://github.com/router-for-me/CLIProxyAPI/blob/fd48ea6840f5572deb53aeb5657740937ac9daaa/internal/api/handlers/management/auth_files_v8.go#L10-L35) establish these paths. Validate exact request/response projections when implementing the AO adapter; avoid exposing the complete credential response.

Important source details:

- `is_webui` callback forwarding binds `0.0.0.0`; [forwarder](https://github.com/router-for-me/CLIProxyAPI/blob/fd48ea6840f5572deb53aeb5657740937ac9daaa/internal/api/handlers/management/auth_files_oauth_callback.go#L17-L56). Direct SDK OAuth servers also use all-interface binds: [Codex](https://github.com/router-for-me/CLIProxyAPI/blob/fd48ea6840f5572deb53aeb5657740937ac9daaa/internal/auth/codex/oauth_server.go#L82-L98), [Claude](https://github.com/router-for-me/CLIProxyAPI/blob/fd48ea6840f5572deb53aeb5657740937ac9daaa/internal/auth/claude/oauth_server.go#L85-L101).
- Provider redirect URIs use fixed callback ports: [Codex](https://github.com/router-for-me/CLIProxyAPI/blob/fd48ea6840f5572deb53aeb5657740937ac9daaa/internal/auth/codex/openai_auth.go#L23-L28), [Claude](https://github.com/router-for-me/CLIProxyAPI/blob/fd48ea6840f5572deb53aeb5657740937ac9daaa/internal/auth/claude/anthropic_auth.go#L22-L35).
- OAuth completion is success status without credential identity: [status handler](https://github.com/router-for-me/CLIProxyAPI/blob/fd48ea6840f5572deb53aeb5657740937ac9daaa/internal/api/handlers/management/auth_files_provider_oauth.go#L952-L973).
- Delete removes local saved/runtime credentials: [CRUD](https://github.com/router-for-me/CLIProxyAPI/blob/fd48ea6840f5572deb53aeb5657740937ac9daaa/internal/api/handlers/management/auth_files_crud.go#L347-L379). No established-account provider revoke/logout endpoint was found in the inspected production code; that absence is a search finding, not a claim that providers lack revocation facilities.
- Removing one account can close execution sessions at provider-executor scope: [manager removal](https://github.com/router-for-me/CLIProxyAPI/blob/fd48ea6840f5572deb53aeb5657740937ac9daaa/sdk/cliproxy/auth/conductor_lifecycle.go#L294-L351).
- Auth directory defaults and token-file permissions require an AO-owned private root: [directory creation](https://github.com/router-for-me/CLIProxyAPI/blob/fd48ea6840f5572deb53aeb5657740937ac9daaa/sdk/cliproxy/service_lifecycle.go#L359-L364), [file writes](https://github.com/router-for-me/CLIProxyAPI/blob/fd48ea6840f5572deb53aeb5657740937ac9daaa/sdk/auth/filestore.go#L104-L150).
- Upstream is distributed under its [MIT license](https://github.com/router-for-me/CLIProxyAPI/blob/fd48ea6840f5572deb53aeb5657740937ac9daaa/LICENSE); retain license/notices in packaged artifacts.

## Checks actually performed

1. **External public SDK compile passed**, using Go `1.26.4` and a temporary independent module with a local `replace` to the reference clone. Imports include public `/v8/sdk/config`, `/v8/sdk/api`, `/v8/sdk/api/handlers` and `/v8/sdk/cliproxy`. Current exported option functions are in `sdk/api`; the older SDK guide's `cliproxy.WithMiddleware` spelling is stale.
2. **Standard Responses HTTP/SSE routing probe passed**, using public SDK handlers, `httptest`, two fake auth records, registered fake models and a fake provider executor. Ten subcases covered repeated A/B exact routing, replacing a spoofed routing header, unknown session token, missing account, disabled account and quota failure with retries configured. No case executed another account as fallback. The probe uses middleware-stamped request headers, not a replacement protocol handler.
3. **Four upstream focused tests passed:** `TestExecuteStreamWithAuthManager_PinnedAuthKeepsSameUpstream`, `TestExecuteModelPropagatesForcedProviderAndAuthID`, `TestExecuteModelStreamPropagatesForcedProviderAndAuthID`, and `TestExecuteModelPinsExactAuthAcrossPrioritiesWithoutFallback`.
4. **Document checks passed:** all five Mermaid diagrams parsed with AO's installed Mermaid library; local Markdown links, code fences and whitespace were checked.

Planning update on 2 October 2026: all eleven product questions were answered and an implementation plan was added. The updated documents passed parsing for all seven Mermaid diagrams, checks for fourteen local links, balanced code fences and `git diff --check`. No additional production code, real-account tests or complete AO CI runs accompanied that planning update.

Temporary probe source: [routing_test.go](/tmp/ao-cliproxyapi-sdk-proof/routing_test.go), [compile probe](/tmp/ao-cliproxyapi-sdk-proof/main.go). These are research files outside the AO checkout.

Commands used after fetching dependencies into temporary caches:

```sh
cd /tmp/ao-cliproxyapi-sdk-proof
GOCACHE=/tmp/ao-cliproxyapi-build-cache \
GOMODCACHE=/tmp/ao-cliproxyapi-module-cache \
go build -o /tmp/ao-cliproxyapi-sdk-proof/probe .

GOCACHE=/tmp/ao-cliproxyapi-build-cache \
GOMODCACHE=/tmp/ao-cliproxyapi-module-cache \
go test -v .

cd /tmp/ao-cliproxyapi-reference-20261001
GOCACHE=/tmp/ao-cliproxyapi-build-cache \
GOMODCACHE=/tmp/ao-cliproxyapi-module-cache \
go test -v ./sdk/api/handlers -count=1 \
  -run '^(TestExecuteStreamWithAuthManager_PinnedAuthKeepsSameUpstream|TestExecuteModelPropagatesForcedProviderAndAuthID|TestExecuteModelStreamPropagatesForcedProviderAndAuthID|TestExecuteModelPinsExactAuthAcrossPrioritiesWithoutFallback)$'
```

## Explicit limits of this research

The HTTP/SSE probe did not run the full SDK service lifecycle, watcher/config reload, real OAuth/refresh or native Codex/Claude clients. Selector persistence is source-verified and needs a service-level regression test. Concurrent routing, daemon-replacement survival, helper packaging, live account switching, tools, compaction and remote continuation need implementation/live-spike verification. No memory/binary-size benchmark or complete AO CI run was performed; AO production code is unchanged. Production LOC estimates are forecasts.
