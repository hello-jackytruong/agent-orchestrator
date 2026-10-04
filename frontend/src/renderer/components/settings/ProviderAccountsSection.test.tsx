import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ProviderAccountsSection } from "./ProviderAccountsSection";
import type { ProviderAccount, ProviderAccounts } from "../../hooks/useProviderAccounts";

const mock = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn(), put: vi.fn(), remove: vi.fn(), open: vi.fn() }));
vi.mock("../../lib/api-client", () => ({ apiClient: { GET: mock.get, POST: mock.post, PUT: mock.put, DELETE: mock.remove }, apiErrorMessage: (error: { message: string }) => error.message }));
vi.mock("../../lib/bridge", () => ({ aoBridge: { app: { openExternal: mock.open } } }));
let inventory: ProviderAccounts;
function renderAccounts() {
	const cache = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
	render(<QueryClientProvider client={cache}><ProviderAccountsSection /></QueryClientProvider>);
	return cache;
}
function accountRow(email: string) {
	return screen.getByText(email).parentElement!.parentElement!;
}
const alice: ProviderAccount = { id: "a", provider: "codex", email: "alice@example.test", signedIn: true, primary: true, sessions: ["session-a"] };
const bob: ProviderAccount = { id: "b", provider: "codex", email: "bob@example.test", signedIn: true, primary: false, sessions: ["session-b", "session-c"] };
const clara: ProviderAccount = { id: "c", provider: "claude", email: "clara@example.test", signedIn: true, primary: true, sessions: [] };
beforeEach(() => {
	vi.clearAllMocks();
	inventory = { accounts: [structuredClone(alice), structuredClone(bob), structuredClone(clara)], defaults: [{ provider: "codex", primaryId: "a", managed: true }, { provider: "claude", primaryId: "c", managed: true }], recoveryRequired: false };
	mock.get.mockImplementation(async (path: string) => path === "/api/v1/provider-accounts" ? { data: inventory } : { data: { id: "login-1", provider: "codex", url: "https://provider.test/login", status: "waiting", accountId: "" } });
	mock.post.mockResolvedValue({ data: inventory });
	mock.put.mockResolvedValue({ data: inventory });
	mock.remove.mockResolvedValue({ data: inventory });
	mock.open.mockResolvedValue(undefined);
});
afterEach(cleanup);

describe("provider account inventory", () => {
	it("shows separate providers, primaries and session counts", async () => {
		renderAccounts();
		await screen.findByText(alice.email);
		expect(screen.getByRole("heading", { name: "Codex" })).toBeInTheDocument();
		expect(screen.getByRole("heading", { name: "Claude" })).toBeInTheDocument();
		expect(within(accountRow(alice.email)).getByText("Primary · 1 sessions")).toBeInTheDocument();
		expect(within(accountRow(bob.email)).getByText("Signed in · 2 sessions")).toBeInTheDocument();
		expect(within(accountRow(clara.email)).getByText("Primary · 0 sessions")).toBeInTheDocument();
		expect(within(accountRow(alice.email)).queryByRole("button", { name: "Make primary" })).toBeNull();
		expect(screen.getByText(/Existing sessions keep their account/)).toBeInTheDocument();
	});
	it("changes only the selected provider primary and explains existing sessions", async () => {
		const user = userEvent.setup();
		renderAccounts();
		await screen.findByText(bob.email);
		await user.click(within(accountRow(bob.email)).getByRole("button", { name: "Make primary" }));
		expect(mock.put).toHaveBeenCalledWith("/api/v1/provider-accounts/{accountId}/primary", { params: { path: { accountId: "b" } } });
		expect(await screen.findByRole("status")).toHaveTextContent("Primary changed. Existing sessions keep their account.");
		expect(mock.post).not.toHaveBeenCalled();
		expect(mock.remove).not.toHaveBeenCalled();
	});
	it("keeps the catalogue when primary change is refused", async () => {
		mock.put.mockResolvedValue({ error: { message: "Account is signed out" } });
		const user = userEvent.setup(); renderAccounts(); await screen.findByText(bob.email);
		await user.click(within(accountRow(bob.email)).getByRole("button", { name: "Make primary" }));
		expect(await screen.findByRole("status")).toHaveTextContent("Account is signed out");
		expect(screen.getByText(alice.email)).toBeInTheDocument();
	});
	it("shows signed-out entries and supports signing in again", async () => {
		inventory.accounts[1] = { ...bob, signedIn: false, sessions: [] };
		mock.post.mockResolvedValue({ data: { id: "login-1", provider: "codex", url: "https://provider.test/login", status: "waiting", accountId: "b" } });
		const user = userEvent.setup(); renderAccounts(); await screen.findByText(bob.email);
		const row = within(accountRow(bob.email));
		expect(row.getByText("Signed out · 0 sessions")).toBeInTheDocument();
		expect(row.queryByRole("button", { name: "Make primary" })).toBeNull();
		expect(row.queryByRole("button", { name: "Sign out" })).toBeNull();
		await user.click(row.getByRole("button", { name: "Sign in again" }));
		expect(mock.post).toHaveBeenCalledWith("/api/v1/provider-accounts/login", { body: { provider: "codex", accountId: "b" } });
		expect(mock.open).toHaveBeenCalledWith("https://provider.test/login");
	});
	it("distinguishes native device use from an adopted provider waiting for login", async () => {
		inventory.accounts = [];
		inventory.defaults[1].managed = false;
		renderAccounts();
		expect(await screen.findByText(/No signed-in Codex account/)).toHaveTextContent("Managed sessions need you to sign in again.");
		expect(screen.getByText(/No signed-in Claude account/)).toHaveTextContent("Existing device sessions continue using their device account.");
	});
	it("renders recovery instructions without hiding the inventory", async () => {
		inventory.recoveryRequired = true;
		renderAccounts();
		expect(await screen.findByRole("alert")).toHaveTextContent("An account change needs recovery");
		expect(screen.getByText(alice.email)).toBeInTheDocument();
	});
});

describe("account sign-out and removal", () => {
	it("requires choosing another primary before confirming removal", async () => {
		const user = userEvent.setup(); renderAccounts(); await screen.findByText(alice.email);
		await user.click(within(accountRow(alice.email)).getByRole("button", { name: "Remove" }));
		const confirm = screen.getByRole("group", { name: "Confirm account change" });
		expect(within(confirm).getByRole("button", { name: "Confirm" })).toBeDisabled();
		const select = within(confirm).getByRole("combobox", { name: "Replacement primary account" });
		expect(within(select).getAllByRole("option").map(o => o.textContent)).toEqual(["Choose an account", bob.email]);
		await user.selectOptions(select, "b");
		await user.click(within(confirm).getByRole("button", { name: "Confirm" }));
		expect(mock.remove).toHaveBeenCalledWith("/api/v1/provider-accounts/{accountId}", { params: { path: { accountId: "a" } }, body: { replacementPrimaryId: "b" } });
		expect(await screen.findByRole("status")).toHaveTextContent("Account updated");
	});
	it("signs out a secondary after explaining how its sessions move", async () => {
		const user = userEvent.setup(); renderAccounts(); await screen.findByText(bob.email);
		await user.click(within(accountRow(bob.email)).getByRole("button", { name: "Sign out" }));
		const confirm = screen.getByRole("group", { name: "Confirm account change" });
		expect(within(confirm).getByText(/Its sessions will use the primary account/)).toHaveTextContent("If any are busy, wait until they are idle and retry.");
		expect(within(confirm).queryByRole("combobox")).toBeNull();
		await user.click(within(confirm).getByRole("button", { name: "Confirm" }));
		expect(mock.post).toHaveBeenCalledWith("/api/v1/provider-accounts/{accountId}/sign-out", { params: { path: { accountId: "b" } }, body: { replacementPrimaryId: undefined } });
		expect(mock.remove).not.toHaveBeenCalled();
	});
	it("explains last-account login requirements and recovery before removing", async () => {
		inventory.accounts = [structuredClone(clara)];
		const user = userEvent.setup(); renderAccounts(); await screen.findByText(clara.email);
		await user.click(within(accountRow(clara.email)).getByRole("button", { name: "Remove" }));
		const confirm = screen.getByRole("group", { name: "Confirm account change" });
		expect(within(confirm).getByText(/With no account left/)).toHaveTextContent("The next sign-in becomes primary and restores those waiting sessions.");
		expect(within(confirm).queryByRole("combobox")).toBeNull();
		expect(within(confirm).getByRole("button", { name: "Confirm" })).toBeEnabled();
	});
	it("cancelling an account removal performs no destructive request", async () => {
		const user = userEvent.setup(); renderAccounts(); await screen.findByText(bob.email);
		await user.click(within(accountRow(bob.email)).getByRole("button", { name: "Remove" }));
		await user.click(within(screen.getByRole("group", { name: "Confirm account change" })).getByRole("button", { name: "Cancel" }));
		expect(screen.queryByRole("group", { name: "Confirm account change" })).toBeNull();
		expect(mock.remove).not.toHaveBeenCalled();
	});
	it("keeps the confirmation and account after a busy-session refusal", async () => {
		mock.remove.mockResolvedValue({ error: { message: "Some sessions are busy. Wait until idle." } });
		const user = userEvent.setup(); renderAccounts(); await screen.findByText(bob.email);
		await user.click(within(accountRow(bob.email)).getByRole("button", { name: "Remove" }));
		await user.click(within(screen.getByRole("group", { name: "Confirm account change" })).getByRole("button", { name: "Confirm" }));
		expect(await screen.findByRole("status")).toHaveTextContent("Some sessions are busy. Wait until idle.");
		expect(screen.getByRole("group", { name: "Confirm account change" })).toBeInTheDocument();
		expect(screen.getAllByText(bob.email)).not.toHaveLength(0);
	});
});

describe("provider browser login", () => {
	it("opens the provider link and allows reopening and cancelling", async () => {
		mock.post.mockResolvedValue({ data: { id: "login-1", provider: "codex", url: "https://provider.test/login", status: "waiting", accountId: "" } });
		const user = userEvent.setup(); renderAccounts(); await screen.findByText(alice.email);
		await user.click(screen.getAllByRole("button", { name: "Add account" })[0]);
		expect(mock.post).toHaveBeenCalledWith("/api/v1/provider-accounts/login", { body: { provider: "codex", accountId: undefined } });
		expect(await screen.findByText("Complete sign-in in your browser.")).toBeInTheDocument();
		expect(screen.getAllByRole("button", { name: "Add account" }).every(b => (b as HTMLButtonElement).disabled)).toBe(true);
		await user.click(screen.getByRole("button", { name: "Open sign-in" }));
		expect(mock.open).toHaveBeenCalledTimes(2);
		await user.click(screen.getByRole("button", { name: "Cancel" }));
		expect(mock.remove).toHaveBeenCalledWith("/api/v1/provider-accounts/login/{loginId}", { params: { path: { loginId: "login-1" } } });
		await waitFor(() => expect(screen.queryByText("Complete sign-in in your browser.")).toBeNull());
	});
	it("reports a callback-port conflict and keeps Add account available", async () => {
		mock.post.mockResolvedValue({ error: { message: "Login callback port is in use; retry later" } });
		const user = userEvent.setup(); renderAccounts(); await screen.findByText(alice.email);
		await user.click(screen.getAllByRole("button", { name: "Add account" })[1]);
		expect(await screen.findByRole("status")).toHaveTextContent("Login callback port is in use");
		expect(mock.open).not.toHaveBeenCalled();
		expect(screen.getAllByRole("button", { name: "Add account" })[1]).toBeEnabled();
	});
	it("keeps the login link available if the external browser fails to open", async () => {
		mock.post.mockResolvedValue({ data: { id: "login-1", provider: "codex", url: "https://provider.test/login", status: "waiting", accountId: "" } });
		mock.open.mockRejectedValue(new Error("Browser could not be opened"));
		const user = userEvent.setup(); renderAccounts(); await screen.findByText(alice.email);
		await user.click(screen.getAllByRole("button", { name: "Add account" })[0]);
		expect(await screen.findByRole("status")).toHaveTextContent("Browser could not be opened");
		expect(screen.getByRole("button", { name: "Open sign-in" })).toBeEnabled();
	});
});

describe("pending login continuity", () => {
	it("resumes the same browser login after leaving and returning to settings", async () => {
		const cache = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: Infinity } } });
		const user = userEvent.setup();
		mock.post.mockResolvedValue({ data: { id: "login-1", provider: "codex", url: "https://provider.test/login", status: "waiting", accountId: "" } });
		const first = render(<QueryClientProvider client={cache}><ProviderAccountsSection /></QueryClientProvider>);
		await screen.findByText(alice.email);
		await user.click(screen.getAllByRole("button", { name: "Add account" })[0]);
		await screen.findByText("Complete sign-in in your browser.");
		expect(mock.open).toHaveBeenCalledTimes(1);
		expect(mock.post).toHaveBeenCalledTimes(1);
		first.unmount();
		render(<QueryClientProvider client={cache}><ProviderAccountsSection /></QueryClientProvider>);
		expect(screen.getByText("Complete sign-in in your browser.")).toBeInTheDocument();
		expect(screen.getByRole("button", { name: "Open sign-in" })).toBeEnabled();
		expect(screen.getAllByRole("button", { name: "Add account" }).every(button => (button as HTMLButtonElement).disabled)).toBe(true);
		await user.click(screen.getByRole("button", { name: "Open sign-in" }));
		expect(mock.open).toHaveBeenCalledTimes(2);
		expect(mock.post).toHaveBeenCalledTimes(1);
		await user.click(screen.getByRole("button", { name: "Cancel" }));
		await waitFor(() => expect(screen.queryByText("Complete sign-in in your browser.")).toBeNull());
		expect(cache.getQueryData(["provider-account-login"])).toBeNull();
	});
	it("records successful polling in the catalogue and clears waiting controls", async () => {
		const cache = renderAccounts();
		const user = userEvent.setup();
		await screen.findByText(alice.email);
		mock.post.mockResolvedValue({ data: { id: "login-1", provider: "claude", url: "https://provider.test/login", status: "waiting", accountId: "" } });
		mock.get.mockImplementation(async (path: string) => path === "/api/v1/provider-accounts"
			? { data: inventory }
			: { data: { id: "login-1", provider: "claude", url: "https://provider.test/login", status: "complete", accountId: "new-account" } });
		await user.click(screen.getAllByRole("button", { name: "Add account" })[1]);
		await screen.findByText("Complete sign-in in your browser.");
		await waitFor(() => expect(screen.getByRole("status")).toHaveTextContent("Account signed in."), { timeout: 3000 });
		expect(screen.queryByRole("button", { name: "Open sign-in" })).toBeNull();
		expect(screen.getAllByRole("button", { name: "Add account" }).every(button => !(button as HTMLButtonElement).disabled)).toBe(true);
		expect(cache.getQueryData(["provider-account-login"])).toMatchObject({ status: "complete", accountId: "new-account" });
		expect(mock.get).toHaveBeenCalledWith("/api/v1/provider-accounts/login/{loginId}", { params: { path: { loginId: "login-1" } } });
		expect(mock.remove).not.toHaveBeenCalled();
	});
	it("does not announce a successful login after the provider reports failure", async () => {
		const user = userEvent.setup();
		mock.post.mockResolvedValue({ data: { id: "login-1", provider: "codex", url: "https://provider.test/login", status: "waiting", accountId: "" } });
		mock.get.mockImplementation(async (path: string) => path === "/api/v1/provider-accounts"
			? { data: inventory }
			: { data: { id: "login-1", provider: "codex", url: "https://provider.test/login", status: "failed", accountId: "" } });
		renderAccounts();
		await screen.findByText(alice.email);
		await user.click(screen.getAllByRole("button", { name: "Add account" })[0]);
		await screen.findByText("Complete sign-in in your browser.");
		await waitFor(() => expect(screen.getByRole("status")).toHaveTextContent("Sign-in failed. Please sign in again."), { timeout: 3000 });
		expect(screen.queryByText(/Account signed in/)).toBeNull();
		expect(screen.queryByRole("button", { name: "Open sign-in" })).toBeNull();
		expect(screen.getByText(alice.email)).toBeInTheDocument();
		expect(mock.put).not.toHaveBeenCalled();
		expect(mock.remove).not.toHaveBeenCalled();
	});
	it("preserves a pending attempt if cancellation is refused", async () => {
		const user = userEvent.setup();
		mock.post.mockResolvedValue({ data: { id: "login-1", provider: "codex", url: "https://provider.test/login", status: "waiting", accountId: "" } });
		mock.remove.mockResolvedValue({ error: { message: "Unable to cancel login. Try again." } });
		const cache = renderAccounts();
		await screen.findByText(alice.email);
		await user.click(screen.getAllByRole("button", { name: "Add account" })[0]);
		await screen.findByText("Complete sign-in in your browser.");
		await user.click(screen.getByRole("button", { name: "Cancel" }));
		expect(await screen.findByRole("status")).toHaveTextContent("Unable to cancel login. Try again.");
		expect(screen.getByText("Complete sign-in in your browser.")).toBeInTheDocument();
		expect(cache.getQueryData(["provider-account-login"])).toMatchObject({ id: "login-1", status: "waiting" });
		expect(screen.getByRole("button", { name: "Open sign-in" })).toBeEnabled();
		expect(mock.post).toHaveBeenCalledTimes(1);
	});
	it("retains the current attempt and retry controls after a poll transport failure", async () => {
		const user = userEvent.setup();
		mock.post.mockResolvedValue({ data: { id: "login-1", provider: "codex", url: "https://provider.test/login", status: "waiting", accountId: "" } });
		mock.get.mockImplementation(async (path: string) => path === "/api/v1/provider-accounts"
			? { data: inventory }
			: { error: { message: "AO is reconnecting" } });
		const cache = renderAccounts();
		await screen.findByText(alice.email);
		await user.click(screen.getAllByRole("button", { name: "Add account" })[0]);
		await screen.findByText("Complete sign-in in your browser.");
		await waitFor(() => expect(screen.getByRole("status")).toHaveTextContent("AO is reconnecting"), { timeout: 3000 });
		expect(cache.getQueryData(["provider-account-login"])).toMatchObject({ status: "waiting" });
		expect(screen.getByRole("button", { name: "Cancel" })).toBeEnabled();
		expect(mock.post).toHaveBeenCalledTimes(1);
		expect(mock.remove).not.toHaveBeenCalled();
	});
});

describe("catalogue and confirmation failure boundaries", () => {
	it("shows a catalogue error without pretending that all accounts were signed out", async () => {
		mock.get.mockResolvedValue({ error: { message: "Account catalogue unavailable" } });
		renderAccounts();
		expect(await screen.findByRole("alert", {}, { timeout: 3000 })).toHaveTextContent("Account catalogue unavailable");
		expect(screen.queryByText(/No signed-in Codex account/)).toBeNull();
		expect(screen.queryByText(/No signed-in Claude account/)).toBeNull();
		expect(screen.queryByText(alice.email)).toBeNull();
		expect(mock.remove).not.toHaveBeenCalled();
		expect(mock.put).not.toHaveBeenCalled();
	});
	it("excludes signed-out and other-provider accounts from replacement choices", async () => {
		inventory.accounts.push({ id: "d", provider: "codex", email: "signed-out@example.test", signedIn: false, primary: false, sessions: [] });
		const user = userEvent.setup();
		renderAccounts();
		await screen.findByText(alice.email);
		await user.click(within(accountRow(alice.email)).getByRole("button", { name: "Sign out" }));
		const options = screen.getAllByRole("option").map(option => option.textContent);
		expect(options).toEqual(["Choose an account", bob.email]);
		expect(options).not.toContain(clara.email);
		expect(options).not.toContain("signed-out@example.test");
		expect(screen.getByRole("button", { name: "Confirm" })).toBeDisabled();
	});
	it("signs out a primary only with the replacement explicitly selected by the user", async () => {
		const user = userEvent.setup();
		renderAccounts();
		await screen.findByText(alice.email);
		await user.click(within(accountRow(alice.email)).getByRole("button", { name: "Sign out" }));
		expect(screen.getByRole("button", { name: "Confirm" })).toBeDisabled();
		await user.selectOptions(screen.getByRole("combobox", { name: "Replacement primary account" }), "b");
		await user.click(screen.getByRole("button", { name: "Confirm" }));
		expect(mock.post).toHaveBeenCalledWith("/api/v1/provider-accounts/{accountId}/sign-out", { params: { path: { accountId: "a" } }, body: { replacementPrimaryId: "b" } });
		expect(mock.remove).not.toHaveBeenCalled();
		expect(mock.put).not.toHaveBeenCalled();
		expect(await screen.findByRole("status")).toHaveTextContent("Account updated.");
	});
	it("keeps the catalogue and confirmation on credential deletion failure", async () => {
		mock.remove.mockResolvedValue({ error: { message: "Credential cleanup failed. Retry the operation." } });
		const user = userEvent.setup();
		renderAccounts();
		await screen.findByText(bob.email);
		await user.click(within(accountRow(bob.email)).getByRole("button", { name: "Remove" }));
		await user.click(screen.getByRole("button", { name: "Confirm" }));
		expect(await screen.findByRole("status")).toHaveTextContent("Credential cleanup failed.");
		expect(screen.getByRole("group", { name: "Confirm account change" })).toBeInTheDocument();
		expect(screen.getByText(alice.email)).toBeInTheDocument();
		expect(screen.queryByText(/^Account updated/)).toBeNull();
		expect(mock.post).not.toHaveBeenCalled();
	});
	it("prevents duplicate destructive requests while one is pending", async () => {
		let resolve!: (value: { data: ProviderAccounts }) => void;
		mock.remove.mockImplementation(() => new Promise<{ data: ProviderAccounts }>(done => { resolve = done; }));
		const user = userEvent.setup();
		renderAccounts();
		await screen.findByText(bob.email);
		await user.click(within(accountRow(bob.email)).getByRole("button", { name: "Remove" }));
		await user.click(screen.getByRole("button", { name: "Confirm" }));
		expect(screen.getByRole("button", { name: "Confirm" })).toBeDisabled();
		await user.click(screen.getByRole("button", { name: "Confirm" }));
		expect(mock.remove).toHaveBeenCalledTimes(1);
		expect(within(accountRow(alice.email)).getByRole("button", { name: "Sign out" })).toBeDisabled();
		resolve({ data: inventory });
		await waitFor(() => expect(screen.queryByRole("group", { name: "Confirm account change" })).toBeNull());
		expect(screen.getByRole("status")).toHaveTextContent("Account updated.");
	});
});

describe("login after daemon replacement", () => {
	it("releases an unknown attempt and permits a fresh sign-in without manual cancellation", async () => {
		const cache = new QueryClient({ defaultOptions: { queries: { retry: false } } });
		cache.setQueryData(["provider-account-login"], { id: "old-attempt", provider: "codex", url: "https://provider.test/old", status: "waiting", accountId: "" });
		mock.get.mockImplementation(async (path: string) => path === "/api/v1/provider-accounts"
			? { data: inventory }
			: { error: { code: "PROVIDER_LOGIN_NOT_FOUND", message: "Login attempt not found" } });
		render(<QueryClientProvider client={cache}><ProviderAccountsSection /></QueryClientProvider>);
		await screen.findByText(alice.email);
		await waitFor(() => expect(screen.getByRole("status")).toHaveTextContent("Sign-in failed. Please sign in again."), { timeout: 3000 });
		expect(screen.queryByRole("button", { name: "Open sign-in" })).toBeNull();
		expect(cache.getQueryData(["provider-account-login"])).toMatchObject({ id: "old-attempt", status: "failed" });
		expect(screen.getAllByRole("button", { name: "Add account" }).every(button => !(button as HTMLButtonElement).disabled)).toBe(true);
		expect(mock.remove).not.toHaveBeenCalled();
		expect(mock.open).not.toHaveBeenCalled();
		mock.post.mockResolvedValue({ data: { id: "new-attempt", provider: "codex", url: "https://provider.test/new", status: "waiting", accountId: "" } });
		const user = userEvent.setup();
		await user.click(screen.getAllByRole("button", { name: "Add account" })[0]);
		expect(mock.open).toHaveBeenCalledWith("https://provider.test/new");
		expect(cache.getQueryData(["provider-account-login"])).toMatchObject({ id: "new-attempt", status: "waiting" });
		expect(screen.getByText(alice.email)).toBeInTheDocument();
		expect(mock.put).not.toHaveBeenCalled();
	});
});
