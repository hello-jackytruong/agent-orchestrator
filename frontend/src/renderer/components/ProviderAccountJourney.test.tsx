import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ProviderAccounts, ProviderLogin } from "../hooks/useProviderAccounts";
import type { components } from "../../api/schema";
import { ProviderAccountsSection } from "./settings/ProviderAccountsSection";
import { SessionProviderAccount } from "./SessionProviderAccount";

const api = vi.hoisted(() => ({ get: vi.fn(), put: vi.fn(), post: vi.fn(), remove: vi.fn(), open: vi.fn() }));
vi.mock("../lib/api-client", () => ({
	apiClient: { GET: api.get, PUT: api.put, POST: api.post, DELETE: api.remove },
	apiErrorMessage: (error: { message: string }) => error.message,
}));
vi.mock("../lib/bridge", () => ({ aoBridge: { app: { openExternal: api.open } } }));

type SessionRoute = components["schemas"]["SessionProviderAccountResponse"];
let accounts: ProviderAccounts;
let sessions: Record<string, SessionRoute>;
let busy: boolean;
let login: ProviderLogin;
type PathOptions = { params?: { path?: { sessionId?: string; accountId?: string; loginId?: string } }; body?: { accountId?: string; replacementPrimaryId?: string; provider?: string; moveExisting?: boolean } };

function markPrimary(id: string) {
	const selected = accounts.accounts.find(account => account.id === id)!;
	for (const account of accounts.accounts) {
		if (account.provider === selected.provider) account.primary = account.id === id;
	}
	const defaults = accounts.defaults.find(entry => entry.provider === selected.provider)!;
	defaults.primaryId = id;
}

function sessionCounts() {
	for (const account of accounts.accounts) {
		account.sessions = Object.entries(sessions).filter(([, route]) => route.accountId === account.id).map(([id]) => id);
	}
}

beforeEach(() => {
	vi.resetAllMocks();
	busy = false;
	accounts = {
		accounts: [
			{ id: "alice", provider: "codex", email: "alice@example.test", signedIn: true, primary: true, sessions: ["first"] },
			{ id: "bob", provider: "codex", email: "bob@example.test", signedIn: true, primary: false, sessions: ["second"] },
			{ id: "clara", provider: "claude", email: "clara@example.test", signedIn: true, primary: true, sessions: ["claude"] },
		],
		defaults: [{ provider: "codex", primaryId: "alice", managed: true }, { provider: "claude", primaryId: "clara", managed: true }],
		recoveryRequired: false,
	};
	sessions = {
		first: { managed: true, provider: "codex", accountId: "alice", loginRequired: false },
		second: { managed: true, provider: "codex", accountId: "bob", loginRequired: false },
		claude: { managed: true, provider: "claude", accountId: "clara", loginRequired: false },
		native: { managed: false, provider: "", accountId: "", loginRequired: false },
	};
	login = { id: "attempt", provider: "codex", url: "https://provider.example.test/login", status: "waiting", accountId: "alice" };
	api.get.mockImplementation(async (path: string, options?: PathOptions) => {
		if (path === "/api/v1/provider-accounts") return { data: structuredClone(accounts) };
		if (path === "/api/v1/sessions/{sessionId}/provider-account") return { data: structuredClone(sessions[options!.params!.path!.sessionId!]) };
		if (path === "/api/v1/provider-accounts/login/{loginId}") return { data: structuredClone(login) };
		throw new Error(`Unexpected read: ${path}`);
	});
	api.put.mockImplementation(async (path: string, options: PathOptions) => {
		if (path === "/api/v1/provider-accounts/{accountId}/primary") {
			markPrimary(options.params!.path!.accountId!);
			return { data: structuredClone(accounts) };
		}
		if (path === "/api/v1/sessions/{sessionId}/provider-account") {
			if (busy) return { error: { message: "Wait until this session is idle" } };
			const route = sessions[options.params!.path!.sessionId!]!;
			route.accountId = options.body!.accountId!;
			route.loginRequired = false;
			sessionCounts();
			return { data: structuredClone(route) };
		}
		throw new Error(`Unexpected write: ${path}`);
	});
	api.remove.mockImplementation(async (path: string, options: PathOptions) => {
		if (path === "/api/v1/provider-accounts/login/{loginId}") return {};
		if (busy) return { error: { message: "Some sessions are using this account. Wait until they are idle" } };
		const id = options.params!.path!.accountId!;
		const removed = accounts.accounts.find(account => account.id === id)!;
		const defaults = accounts.defaults.find(entry => entry.provider === removed.provider)!;
		if (removed.primary) {
			defaults.primaryId = options.body?.replacementPrimaryId ?? "";
			if (defaults.primaryId) markPrimary(defaults.primaryId);
		}
		for (const route of Object.values(sessions)) {
			if (route.accountId === id) {
				route.accountId = defaults.primaryId;
				route.loginRequired = !defaults.primaryId;
			}
		}
		accounts.accounts = accounts.accounts.filter(account => account.id !== id);
		sessionCounts();
		return { data: structuredClone(accounts) };
	});
	api.post.mockImplementation(async (path: string, options?: PathOptions) => {
		if (path === "/api/v1/provider-accounts/login") return { data: structuredClone(login) };
		if (path === "/api/v1/provider-accounts/{accountId}/sign-out") {
			const accountId = options?.params?.path?.accountId;
			const account = accounts.accounts.find(candidate => candidate.id === accountId)!;
			account.signedIn = false;
			for (const route of Object.values(sessions)) {
				if (route.accountId === accountId) {
					route.accountId = accounts.defaults.find(entry => entry.provider === account.provider)?.primaryId ?? "";
					route.loginRequired = !route.accountId;
				}
			}
			sessionCounts();
			return { data: structuredClone(accounts) };
		}
		throw new Error(`Unexpected post: ${path}`);
	});
	api.open.mockResolvedValue(undefined);
});
afterEach(cleanup);

function renderJourney() {
	const client = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: Infinity }, mutations: { retry: false } } });
	render(<QueryClientProvider client={client}>
		<section aria-label="Account catalogue"><ProviderAccountsSection /></section>
		<section aria-label="First session"><SessionProviderAccount sessionId="first" /></section>
		<section aria-label="Second session"><SessionProviderAccount sessionId="second" /></section>
		<section aria-label="Claude session"><SessionProviderAccount sessionId="claude" /></section>
		<section aria-label="Native session"><SessionProviderAccount sessionId="native" /></section>
	</QueryClientProvider>);
	return client;
}

function catalogue() {
	return within(screen.getByRole("region", { name: "Account catalogue" }));
}
function row(email: string) {
	return within(catalogue().getByText(email).closest<HTMLElement>('[data-testid^="provider-account-"]')!);
}
function picker(session: string) {
	return within(screen.getByRole("region", { name: session })).getByRole("combobox", { name: "Session account" });
}

describe("account settings and live session controls together", () => {
	it("changes the new-session primary while both existing Codex pins and Claude stay unchanged", async () => {
		const user = userEvent.setup();
		renderJourney();
		await catalogue().findByText("bob@example.test");
		await waitFor(() => expect(picker("First session")).toHaveValue("alice"));
		expect(picker("Second session")).toHaveValue("bob");
		expect(picker("Claude session")).toHaveValue("clara");
		await user.click(row("bob@example.test").getByRole("button", { name: "Use as default" }));
		const primaryChange = within(screen.getByRole("group", { name: "Confirm default change" }));
		await user.click(primaryChange.getByRole("button", { name: "New sessions only" }));
		await waitFor(() => expect(row("bob@example.test").getByText("Default · 1 sessions")).toBeInTheDocument());
		expect(picker("First session")).toHaveValue("alice");
		expect(picker("Second session")).toHaveValue("bob");
		expect(picker("Claude session")).toHaveValue("clara");
		expect(accounts.defaults.find(entry => entry.provider === "codex")?.primaryId).toBe("bob");
		expect(accounts.defaults.find(entry => entry.provider === "claude")?.primaryId).toBe("clara");
		expect(api.put).toHaveBeenCalledTimes(1);
		expect(api.put).toHaveBeenCalledWith("/api/v1/provider-accounts/{accountId}/primary", { params: { path: { accountId: "bob" } }, body: { moveExisting: false } });
		expect(api.remove).not.toHaveBeenCalled();
		expect(api.post).not.toHaveBeenCalled();
		expect(within(screen.getByRole("region", { name: "Native session" })).queryByRole("combobox")).toBeNull();
	});

	it("manually switches one session and refreshes account counts without changing either primary", async () => {
		const user = userEvent.setup();
		renderJourney();
		await catalogue().findByText("bob@example.test");
		await waitFor(() => expect(picker("First session")).toHaveValue("alice"));
		await user.selectOptions(picker("First session"), "bob");
		await waitFor(() => expect(picker("First session")).toHaveValue("bob"));
		await waitFor(() => expect(row("bob@example.test").getByText("Signed in · 2 sessions")).toBeInTheDocument());
		expect(row("alice@example.test").getByText("Default · 0 sessions")).toBeInTheDocument();
		expect(picker("Second session")).toHaveValue("bob");
		expect(picker("Claude session")).toHaveValue("clara");
		expect(accounts.defaults[0].primaryId).toBe("alice");
		expect(accounts.defaults[1].primaryId).toBe("clara");
		expect(api.put).toHaveBeenCalledTimes(1);
		expect(api.put).toHaveBeenCalledWith("/api/v1/sessions/{sessionId}/provider-account", { params: { path: { sessionId: "first" } }, body: { accountId: "bob" } });
		expect(api.post).not.toHaveBeenCalled();
		expect(api.remove).not.toHaveBeenCalled();
	});

	it("keeps a busy manual switch unchanged and requires a fresh explicit retry", async () => {
		const user = userEvent.setup();
		const cache = renderJourney();
		await catalogue().findByText("alice@example.test");
		await waitFor(() => expect(picker("First session")).toHaveValue("alice"));
		busy = true;
		await user.selectOptions(picker("First session"), "bob");
		await waitFor(() => expect(within(screen.getByRole("region", { name: "First session" })).getByRole("status")).toHaveTextContent("Wait until this session is idle"));
		expect(picker("First session")).toHaveValue("alice");
		expect(picker("Second session")).toHaveValue("bob");
		expect(api.put).toHaveBeenCalledTimes(1);
		busy = false;
		await cache.invalidateQueries({ queryKey: ["provider-accounts"] });
		await cache.invalidateQueries({ queryKey: ["session-provider-account", "first"] });
		expect(picker("First session")).toHaveValue("alice");
		expect(api.put).toHaveBeenCalledTimes(1);
		await user.selectOptions(picker("First session"), "bob");
		await waitFor(() => expect(picker("First session")).toHaveValue("bob"));
		expect(api.put).toHaveBeenCalledTimes(2);
		expect(picker("Claude session")).toHaveValue("clara");
		expect(accounts.defaults[0].primaryId).toBe("alice");
	});

	it("signs out and removes a secondary after its sessions move to the default", async () => {
		const user = userEvent.setup();
		const cache = renderJourney();
		await catalogue().findByText("bob@example.test");
		await waitFor(() => expect(picker("Second session")).toHaveValue("bob"));
		await user.click(row("bob@example.test").getByRole("button", { name: "Sign out" }));
		await user.click(within(screen.getByRole("group", { name: "Confirm account change" })).getByRole("button", { name: "Confirm" }));
		await waitFor(() => expect(row("bob@example.test").getByRole("button", { name: "Remove" })).toBeInTheDocument());
		await user.click(row("bob@example.test").getByRole("button", { name: "Remove" }));
		const confirmation = within(screen.getByRole("group", { name: "Confirm account change" }));
		expect(confirmation.getByText("No sessions are currently assigned to this account.")).toBeInTheDocument();
		expect(confirmation.queryByRole("combobox")).toBeNull();
		await user.click(confirmation.getByRole("button", { name: "Confirm" }));
		await waitFor(() => expect(catalogue().queryByText("bob@example.test")).toBeNull());
		await cache.invalidateQueries({ queryKey: ["session-provider-account"] });
		await waitFor(() => expect(picker("Second session")).toHaveValue("alice"));
		expect(picker("First session")).toHaveValue("alice");
		expect(picker("Claude session")).toHaveValue("clara");
		expect(row("alice@example.test").getByText("Default · 2 sessions")).toBeInTheDocument();
		expect(api.remove).toHaveBeenCalledTimes(1);
		expect(api.post).toHaveBeenCalledTimes(1);
		expect(api.put).not.toHaveBeenCalled();
	});

	it("asks for a replacement when removing a primary and changes only its affected provider routes", async () => {
		accounts.accounts[0] = { ...accounts.accounts[0], signedIn: false };
		const user = userEvent.setup();
		const cache = renderJourney();
		await catalogue().findByText("alice@example.test");
		await user.click(row("alice@example.test").getByRole("button", { name: "Remove" }));
		const group = within(screen.getByRole("group", { name: "Confirm account change" }));
		expect(group.getByRole("button", { name: "Confirm" })).toBeDisabled();
		const choices = group.getByRole("combobox", { name: "Replacement default account" });
		expect(within(choices).getAllByRole("option").map(option => option.textContent)).toEqual(["Choose an account", "bob@example.test"]);
		await user.selectOptions(choices, "bob");
		await user.click(group.getByRole("button", { name: "Confirm" }));
		await waitFor(() => expect(catalogue().queryByText("alice@example.test")).toBeNull());
		await cache.invalidateQueries({ queryKey: ["session-provider-account"] });
		await waitFor(() => expect(picker("First session")).toHaveValue("bob"));
		expect(picker("Second session")).toHaveValue("bob");
		expect(picker("Claude session")).toHaveValue("clara");
		expect(row("bob@example.test").getByText("Default · 2 sessions")).toBeInTheDocument();
		expect(accounts.defaults[0].primaryId).toBe("bob");
		expect(accounts.defaults[1].primaryId).toBe("clara");
		expect(api.remove).toHaveBeenCalledWith("/api/v1/provider-accounts/{accountId}", { params: { path: { accountId: "alice" } }, body: { replacementPrimaryId: "bob" } });
	});

	it("leaves managed sessions waiting after the last account is removed and restores them after sign-in", async () => {
		accounts.accounts = accounts.accounts.filter(account => account.id !== "bob");
		accounts.accounts[0] = { ...accounts.accounts[0], signedIn: false };
		sessions.second.accountId = "alice";
		sessionCounts();
		const user = userEvent.setup();
		const cache = renderJourney();
		await catalogue().findByText("alice@example.test");
		await user.click(row("alice@example.test").getByRole("button", { name: "Remove" }));
		expect(screen.getByText(/With no account left/)).toHaveTextContent("The next sign-in becomes the default and restores those waiting sessions.");
		await user.click(screen.getByRole("button", { name: "Confirm" }));
		await waitFor(() => expect(catalogue().queryByText("alice@example.test")).toBeNull());
		await cache.invalidateQueries({ queryKey: ["session-provider-account"] });
		await waitFor(() => expect(picker("First session")).toHaveValue(""));
		expect(picker("Second session")).toHaveValue("");
		expect(screen.getByText("Managed sessions need you to sign in again.")).toBeInTheDocument();
		expect(picker("Claude session")).toHaveValue("clara");
		expect(api.put).not.toHaveBeenCalled();
		await user.click(screen.getAllByRole("button", { name: "Add account" })[0]);
		await user.click(screen.getByRole("button", { name: "Browser" }));
		await screen.findByText("Complete sign-in in your browser.");
		expect(api.open).toHaveBeenCalledWith("https://provider.example.test/login");
		accounts.accounts.push({ id: "new-codex", provider: "codex", email: "new@example.test", signedIn: true, primary: true, sessions: ["first", "second"] });
		accounts.defaults[0].primaryId = "new-codex";
		sessions.first = { ...sessions.first, accountId: "new-codex", loginRequired: false };
		sessions.second = { ...sessions.second, accountId: "new-codex", loginRequired: false };
		login = { ...login, status: "complete", accountId: "new-codex" };
		await waitFor(() => expect(catalogue().getByText("new@example.test")).toBeInTheDocument(), { timeout: 3000 });
		await cache.invalidateQueries({ queryKey: ["session-provider-account"] });
		await waitFor(() => expect(picker("First session")).toHaveValue("new-codex"));
		expect(picker("Second session")).toHaveValue("new-codex");
		expect(picker("Claude session")).toHaveValue("clara");
		expect(api.post).toHaveBeenCalledTimes(1);
		expect(api.put).not.toHaveBeenCalled();
	});
});
