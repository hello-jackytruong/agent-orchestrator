import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ProviderAccountsSection } from "./ProviderAccountsSection";
import type { ProviderAccount, ProviderAccounts } from "../../hooks/useProviderAccounts";

const mock = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn(), put: vi.fn(), patch: vi.fn(), remove: vi.fn(), open: vi.fn(), clipboard: vi.fn() }));
vi.mock("../../lib/api-client", () => ({ apiClient: { GET: mock.get, POST: mock.post, PUT: mock.put, PATCH: mock.patch, DELETE: mock.remove }, apiErrorMessage: (error: { message: string }) => error.message }));
vi.mock("../../lib/bridge", () => ({ aoBridge: { app: { openExternal: mock.open }, clipboard: { writeText: mock.clipboard } } }));
let inventory: ProviderAccounts;
function renderAccounts() {
	const cache = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
	render(<QueryClientProvider client={cache}><ProviderAccountsSection /></QueryClientProvider>);
	return cache;
}
function accountRow(email: string) {
	return screen.getByText(email).closest<HTMLElement>('[data-testid^="provider-account-"]')!;
}
const alice: ProviderAccount = { id: "a", provider: "codex", displayName: "Cedar Codex", email: "alice@example.test", signedIn: true, primary: true, sessions: ["session-a"] };
const bob: ProviderAccount = { id: "b", provider: "codex", displayName: "Maple Codex", email: "bob@example.test", signedIn: true, primary: false, sessions: ["session-b", "session-c"] };
const clara: ProviderAccount = { id: "c", provider: "claude", displayName: "Willow Claude", email: "clara@example.test", signedIn: true, primary: true, sessions: [] };
beforeEach(() => {
	vi.clearAllMocks();
	inventory = { accounts: [structuredClone(alice), structuredClone(bob), structuredClone(clara)], defaults: [{ provider: "codex", primaryId: "a", managed: true }, { provider: "claude", primaryId: "c", managed: true }], recoveryRequired: false, codexQuotaAutoSwitch: false, claudeQuotaAutoSwitch: false };
	mock.get.mockImplementation(async (path: string) => path === "/api/v1/provider-accounts" ? { data: inventory } : { data: { id: "login-1", provider: "codex", url: "https://provider.test/login", status: "waiting", accountId: "" } });
	mock.post.mockResolvedValue({ data: inventory });
	mock.put.mockResolvedValue({ data: inventory });
	mock.patch.mockResolvedValue({ data: { ...inventory, codexQuotaAutoSwitch: true, claudeQuotaAutoSwitch: true } });
	mock.remove.mockResolvedValue({ data: inventory });
	mock.open.mockResolvedValue(undefined);
	mock.clipboard.mockResolvedValue(undefined);
});
afterEach(cleanup);

describe("provider account inventory", () => {
	it("shows separate providers, primaries and session counts", async () => {
		renderAccounts();
		await screen.findByText(alice.email);
		expect(screen.getByRole("heading", { name: "Codex" })).toBeInTheDocument();
		expect(screen.getByRole("heading", { name: "Claude" })).toBeInTheDocument();
		expect(within(accountRow(alice.email)).getByText("Default · 1 sessions")).toBeInTheDocument();
		expect(within(accountRow(bob.email)).getByText("Signed in · 2 sessions")).toBeInTheDocument();
		expect(within(accountRow(clara.email)).getByText("Default · 0 sessions")).toBeInTheDocument();
		expect(within(accountRow(alice.email)).queryByRole("button", { name: "Use as default" })).toBeNull();
		expect(within(accountRow(alice.email)).getByRole("button", { name: "Sign out" })).toBeInTheDocument();
		expect(within(accountRow(alice.email)).queryByRole("button", { name: "Remove" })).toBeNull();
		expect(screen.getByText(/choose whether it affects new sessions only/)).toBeInTheDocument();
	});
	it("renames an account inline while keeping its email private", async () => {
		const user = userEvent.setup();
		const renamed = { ...inventory, accounts: inventory.accounts.map(account => account.id === "a" ? { ...account, displayName: "Work Codex" } : account) };
		mock.patch.mockImplementation(async (path: string) => path === "/api/v1/provider-accounts/{accountId}" ? { data: renamed } : { data: inventory });
		renderAccounts();
		await screen.findByText(alice.displayName);
		const name = screen.getByText(alice.displayName);
		expect(screen.getByText(alice.email)).toHaveClass("blur-sm");
		await user.dblClick(name);
		const input = screen.getByRole("textbox", { name: "Account name" });
		await user.clear(input);
		await user.type(input, "Work Codex");
		await user.keyboard("{Enter}");
		expect(mock.patch).toHaveBeenCalledWith("/api/v1/provider-accounts/{accountId}", { params: { path: { accountId: "a" } }, body: { displayName: "Work Codex" } });
		await screen.findByText("Account name updated.");
	});
	it("shows when an account was discovered from the native provider login", async () => {
		const native = { ...inventory, accounts: inventory.accounts.map(account => account.id === "a" ? { ...account, global: true } : account) };
		mock.get.mockImplementation(async (path: string) => path === "/api/v1/provider-accounts" ? { data: native } : { data: { id: "login-1", provider: "codex", url: "https://provider.test/login", status: "waiting", accountId: "" } });
		renderAccounts();
		expect(await screen.findByText("Global")).toHaveAttribute("title", "Discovered from the provider's native login");
	});
	it("shows provider usage without changing account controls", async () => {
		inventory.accounts[0].usage = { status: "available", plan: "Pro", windows: [{ name: "5 hour", remainingFraction: 0.75, resetTime: "2030-01-01T00:00:00Z" }] };
		inventory.accounts[1].usage = { status: "unavailable", message: "Usage unavailable" };
		inventory.accounts[2].usage = { status: "available", plan: "Claude Pro", windows: [{ name: "5 hour", remainingFraction: 0.75, resetTime: "2030-01-01T00:00:00Z" }, { name: "Weekly", remainingFraction: 0.4, resetTime: "2030-01-07T00:00:00Z" }] };
		renderAccounts();
		await screen.findByText(alice.email);
		expect(screen.getByTestId("provider-account-usage-a")).toHaveTextContent(/Pro · 75% remaining · resets Jan 1/);
		expect(screen.getByTestId("provider-account-usage-a")).not.toHaveTextContent("GMT");
		expect(screen.getByTestId("provider-account-usage-b")).toHaveTextContent("Usage unavailable");
		expect(screen.getByTestId("provider-account-usage-c")).toHaveTextContent(/Claude Pro · 75% remaining/);
		expect(screen.getByTestId("provider-account-usage-c-1")).toHaveTextContent(/40% remaining/);
		expect(screen.getAllByRole("progressbar", { name: /clara@example.test/ })).toHaveLength(2);
	});
	it("collapses and reopens a provider section from its header", async () => {
		const user = userEvent.setup();
		renderAccounts();
		await screen.findByText(alice.email);
		const header = screen.getByRole("button", { name: /Codex.*2 signed in/ });
		const content = screen.getByTestId("provider-section-codex").querySelector("[id='provider-content-codex']")!;
		expect(header).toHaveAttribute("aria-expanded", "true");
		await user.click(header);
		expect(header).toHaveAttribute("aria-expanded", "false");
		expect(content).toHaveAttribute("hidden");
		await user.click(header);
		expect(header).toHaveAttribute("aria-expanded", "true");
		expect(content).not.toHaveAttribute("hidden");
	});
	it("asks whether a default change affects new or existing Codex sessions", async () => {
		const user = userEvent.setup();
		renderAccounts();
		await screen.findByText(bob.email);
		await user.click(within(accountRow(bob.email)).getByRole("button", { name: "Use as default" }));
		const confirm = screen.getByRole("group", { name: "Confirm default change" });
		expect(mock.put).not.toHaveBeenCalled();
		await user.click(within(confirm).getByRole("button", { name: "New sessions only" }));
		expect(mock.put).toHaveBeenCalledWith("/api/v1/provider-accounts/{accountId}/primary", { params: { path: { accountId: "b" } }, body: { moveExisting: false } });
		expect(await screen.findByRole("status")).toHaveTextContent("Default changed. Existing Codex sessions keep their account.");
		expect(within(accountRow(bob.email)).queryByRole("group", { name: "Confirm default change" })).toBeNull();
		expect(mock.post).not.toHaveBeenCalled();
		expect(mock.remove).not.toHaveBeenCalled();
	});
	it("keeps the catalogue when primary change is refused", async () => {
		mock.put.mockResolvedValue({ error: { message: "Account is signed out" } });
		const user = userEvent.setup(); renderAccounts(); await screen.findByText(bob.email);
		await user.click(within(accountRow(bob.email)).getByRole("button", { name: "Use as default" }));
		await user.click(within(screen.getByRole("group", { name: "Confirm default change" })).getByRole("button", { name: "New sessions only" }));
		expect(await screen.findByRole("status")).toHaveTextContent("Account is signed out");
		expect(screen.getByText(alice.email)).toBeInTheDocument();
	});
	it("allows Codex quota auto-switching to be enabled", async () => {
		const user = userEvent.setup();
		renderAccounts();
		const toggle = await screen.findByRole("checkbox", { name: /Automatically switch the Codex default/ });
		await user.click(toggle);
		expect(mock.patch).toHaveBeenCalledWith("/api/v1/provider-accounts/quota-auto-switch", { body: { provider: "codex", enabled: true } });
	});
	it("allows Claude quota auto-switching to be enabled", async () => {
		inventory.accounts.push({ ...clara, id: "d", email: "claude-two@test.example", primary: false });
		const user = userEvent.setup();
		renderAccounts();
		const toggle = await screen.findByRole("checkbox", { name: /Automatically switch the Claude/ });
		await user.click(toggle);
		expect(mock.patch).toHaveBeenCalledWith("/api/v1/provider-accounts/quota-auto-switch", { body: { provider: "claude", enabled: true } });
	});
	it("keeps quota auto-switch visible but disabled until a second Codex account is signed in", async () => {
		inventory.accounts = [structuredClone(alice), structuredClone(clara)];
		renderAccounts();
		const toggle = await screen.findByRole("checkbox", { name: /Automatically switch the Codex default/ });
		expect(toggle).toBeDisabled();
		expect(screen.getByRole("img", { name: "Sign in to a second Codex account to enable automatic switching." })).toBeInTheDocument();
		expect(mock.patch).not.toHaveBeenCalled();
	});
	it("shows signed-out entries and supports signing in again", async () => {
		inventory.accounts[1] = { ...bob, signedIn: false, sessions: [] };
		mock.post.mockResolvedValue({ data: { id: "login-1", provider: "codex", url: "https://provider.test/login", status: "waiting", accountId: "b" } });
		const user = userEvent.setup(); renderAccounts(); await screen.findByText(bob.email);
		const row = within(accountRow(bob.email));
		expect(row.getByText("Signed out · 0 sessions")).toBeInTheDocument();
		expect(row.queryByRole("button", { name: "Use as default" })).toBeNull();
		expect(row.queryByRole("button", { name: "Sign out" })).toBeNull();
		expect(row.getByRole("button", { name: "Remove" })).toBeInTheDocument();
		expect(row.getByRole("button", { name: "Remove" })).toHaveAttribute("title", "Remove");
		await user.click(row.getByRole("button", { name: "Sign in again" }));
		expect(mock.post).toHaveBeenCalledWith("/api/v1/provider-accounts/login", { body: { provider: "codex", accountId: "b" } });
		expect(mock.open).not.toHaveBeenCalled();
		expect(screen.getByRole("button", { name: "Open sign-in" })).toBeEnabled();
	});
	it("distinguishes native device use from an adopted provider waiting for login", async () => {
		inventory.accounts = [];
		inventory.defaults[1].managed = false;
		renderAccounts();
		await screen.findByText(/No signed-in Codex account/);
		expect(screen.getByText("Managed sessions need you to sign in again.")).toBeInTheDocument();
		await screen.findByText(/No signed-in Claude account/);
		expect(screen.getByText("Existing device sessions continue using their device account.")).toBeInTheDocument();
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
		inventory.accounts[0] = { ...alice, signedIn: false };
		const user = userEvent.setup(); renderAccounts(); await screen.findByText(alice.email);
		await user.click(within(accountRow(alice.email)).getByRole("button", { name: "Remove" }));
		const confirm = screen.getByRole("group", { name: "Confirm account change" });
		expect(within(confirm).getByRole("button", { name: "Confirm" })).toBeDisabled();
		const select = within(confirm).getByRole("combobox", { name: "Replacement default account" });
		expect(within(select).getAllByRole("option").map(o => o.textContent)).toEqual(["Choose an account", bob.email]);
		await user.selectOptions(select, "b");
		await user.click(within(confirm).getByRole("button", { name: "Confirm" }));
		expect(mock.remove).toHaveBeenCalledWith("/api/v1/provider-accounts/{accountId}", { params: { path: { accountId: "a" } }, body: { replacementPrimaryId: "b" } });
		expect(await screen.findByRole("status")).toHaveTextContent("Account updated");
	});
	it("signs out a secondary after explaining how its sessions move", async () => {
		mock.post.mockResolvedValue({ data: { ...inventory, accounts: inventory.accounts.map(account => account.id === "b" ? { ...account, signedIn: false, sessions: [] } : account) } });
		const user = userEvent.setup(); renderAccounts(); await screen.findByText(bob.email);
		await user.click(within(accountRow(bob.email)).getByRole("button", { name: "Sign out" }));
		const confirm = screen.getByRole("group", { name: "Confirm account change" });
		expect(within(screen.getByTestId("provider-section-codex")).getByRole("group", { name: "Confirm account change" })).toBe(confirm);
		expect(within(confirm).getByText(/Existing sessions using this account will be transferred to the default account/)).toHaveTextContent("If any are busy, wait until they are idle and retry.");
		expect(within(confirm).queryByRole("combobox")).toBeNull();
		await user.click(within(confirm).getByRole("button", { name: "Confirm" }));
		expect(mock.post).toHaveBeenCalledWith("/api/v1/provider-accounts/{accountId}/sign-out", { params: { path: { accountId: "b" } }, body: { replacementPrimaryId: undefined } });
		expect(mock.remove).not.toHaveBeenCalled();
		const row = within(accountRow(bob.email));
		expect(row.getByRole("button", { name: "Sign in again" })).toBeInTheDocument();
		expect(row.getByRole("button", { name: "Remove" })).toBeInTheDocument();
		expect(row.queryByRole("button", { name: "Sign out" })).toBeNull();
	});
	it("explains last-account login requirements and recovery before removing", async () => {
		inventory.accounts = [{ ...structuredClone(clara), signedIn: false }];
		const user = userEvent.setup(); renderAccounts(); await screen.findByText(clara.email);
		await user.click(within(accountRow(clara.email)).getByRole("button", { name: "Remove" }));
		const confirm = screen.getByRole("group", { name: "Confirm account change" });
		expect(within(confirm).getByText(/With no account left/)).toHaveTextContent("The next sign-in becomes the default and restores those waiting sessions.");
		expect(within(confirm).queryByRole("combobox")).toBeNull();
		expect(within(confirm).getByRole("button", { name: "Confirm" })).toBeEnabled();
	});
	it("cancelling an account removal performs no destructive request", async () => {
		inventory.accounts[1] = { ...bob, signedIn: false };
		const user = userEvent.setup(); renderAccounts(); await screen.findByText(bob.email);
		await user.click(within(accountRow(bob.email)).getByRole("button", { name: "Remove" }));
		await user.click(within(screen.getByRole("group", { name: "Confirm account change" })).getByRole("button", { name: "Cancel" }));
		expect(screen.queryByRole("group", { name: "Confirm account change" })).toBeNull();
		expect(mock.remove).not.toHaveBeenCalled();
	});
	it("keeps the confirmation and account after a busy-session refusal", async () => {
		mock.remove.mockResolvedValue({ error: { message: "Some sessions are busy. Wait until idle." } });
		inventory.accounts[1] = { ...bob, signedIn: false };
		const user = userEvent.setup(); renderAccounts(); await screen.findByText(bob.email);
		await user.click(within(accountRow(bob.email)).getByRole("button", { name: "Remove" }));
		await user.click(within(screen.getByRole("group", { name: "Confirm account change" })).getByRole("button", { name: "Confirm" }));
		expect(await screen.findByRole("status")).toHaveTextContent("Some sessions are busy. Wait until idle.");
		expect(screen.getByRole("group", { name: "Confirm account change" })).toBeInTheDocument();
		expect(screen.getAllByText(bob.email)).not.toHaveLength(0);
	});
});

describe("provider browser login", () => {
	it("keeps the provider link in the panel until the user opens it", async () => {
		mock.post.mockResolvedValue({ data: { id: "login-1", provider: "codex", url: "https://provider.test/login", status: "waiting", accountId: "" } });
		const user = userEvent.setup(); renderAccounts(); await screen.findByText(alice.email);
		await user.click(screen.getAllByRole("button", { name: "Add account" })[0]);
		await user.click(screen.getByRole("button", { name: "Browser" }));
		expect(mock.post).toHaveBeenCalledWith("/api/v1/provider-accounts/login", { body: { provider: "codex" } });
		const group = screen.getByRole("group", { name: "codex sign-in methods" });
		expect(await within(group).findByText("Complete sign-in in your browser.")).toBeInTheDocument();
		expect(screen.getAllByRole("button", { name: "Add account" }).every(b => (b as HTMLButtonElement).disabled)).toBe(true);
		expect(mock.open).not.toHaveBeenCalled();
		await user.click(within(group).getByRole("button", { name: "Copy link" }));
		expect(mock.clipboard).toHaveBeenCalledWith("https://provider.test/login");
		await user.click(screen.getByRole("button", { name: "Open sign-in" }));
		expect(mock.open).toHaveBeenCalledTimes(1);
		await user.click(screen.getByRole("button", { name: "Cancel" }));
		expect(mock.remove).toHaveBeenCalledWith("/api/v1/provider-accounts/login/{loginId}", { params: { path: { loginId: "login-1" } } });
		await waitFor(() => expect(screen.queryByText("Complete sign-in in your browser.")).toBeNull());
	});
	it("reports a callback-port conflict and keeps Add account available", async () => {
		mock.post.mockResolvedValue({ error: { message: "Login callback port is in use; retry later" } });
		const user = userEvent.setup(); renderAccounts(); await screen.findByText(alice.email);
		await user.click(screen.getAllByRole("button", { name: "Add account" })[1]);
		await user.click(screen.getByRole("button", { name: "Browser" }));
		expect(await screen.findByRole("status")).toHaveTextContent("Login callback port is in use");
		expect(mock.open).not.toHaveBeenCalled();
		expect(screen.getAllByRole("button", { name: "Add account" })[1]).toBeEnabled();
	});
	it("keeps the login link available if the external browser fails to open", async () => {
		mock.post.mockResolvedValue({ data: { id: "login-1", provider: "codex", url: "https://provider.test/login", status: "waiting", accountId: "" } });
		mock.open.mockRejectedValue(new Error("Browser could not be opened"));
		const user = userEvent.setup(); renderAccounts(); await screen.findByText(alice.email);
		await user.click(screen.getAllByRole("button", { name: "Add account" })[0]);
		await user.click(screen.getByRole("button", { name: "Browser" }));
		await user.click(screen.getByRole("button", { name: "Open sign-in" }));
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
		await user.click(screen.getByRole("button", { name: "Browser" }));
		await screen.findByText("Complete sign-in in your browser.");
		expect(mock.open).not.toHaveBeenCalled();
		expect(mock.post).toHaveBeenCalledTimes(1);
		first.unmount();
		render(<QueryClientProvider client={cache}><ProviderAccountsSection /></QueryClientProvider>);
		expect(screen.getByText("Complete sign-in in your browser.")).toBeInTheDocument();
		expect(screen.getByRole("button", { name: "Open sign-in" })).toBeEnabled();
		expect(screen.getAllByRole("button", { name: "Add account" }).every(button => (button as HTMLButtonElement).disabled)).toBe(true);
		await user.click(screen.getByRole("button", { name: "Open sign-in" }));
		expect(mock.open).toHaveBeenCalledTimes(1);
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
		await user.click(screen.getByRole("button", { name: "Browser" }));
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
		await user.click(screen.getByRole("button", { name: "Browser" }));
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
		await user.click(screen.getByRole("button", { name: "Browser" }));
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
		await user.click(screen.getByRole("button", { name: "Browser" }));
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
		await user.selectOptions(screen.getByRole("combobox", { name: "Replacement default account" }), "b");
		await user.click(screen.getByRole("button", { name: "Confirm" }));
		expect(mock.post).toHaveBeenCalledWith("/api/v1/provider-accounts/{accountId}/sign-out", { params: { path: { accountId: "a" } }, body: { replacementPrimaryId: "b" } });
		expect(mock.remove).not.toHaveBeenCalled();
		expect(mock.put).not.toHaveBeenCalled();
		expect(await screen.findByRole("status")).toHaveTextContent("Account updated.");
	});
	it("keeps the catalogue and confirmation on credential deletion failure", async () => {
		mock.remove.mockResolvedValue({ error: { message: "Credential cleanup failed. Retry the operation." } });
		inventory.accounts[1] = { ...bob, signedIn: false };
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
		inventory.accounts[1] = { ...bob, signedIn: false };
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
		await user.click(screen.getByRole("button", { name: "Browser" }));
		expect(mock.open).not.toHaveBeenCalled();
		expect(cache.getQueryData(["provider-account-login"])).toMatchObject({ id: "new-attempt", status: "waiting" });
		expect(screen.getByText(alice.email)).toBeInTheDocument();
		expect(mock.put).not.toHaveBeenCalled();
	});
});

describe("additional CLIProxy credential methods", () => {
	it("offers browser, device, API key and JSON import for Codex", async () => {
		const user = userEvent.setup();
		renderAccounts();
		await screen.findByText(alice.email);
		await user.click(screen.getAllByRole("button", { name: "Add account" })[0]);
		const group = screen.getByRole("group", { name: "codex sign-in methods" });
		expect(group).toBeInTheDocument();
		expect(within(group).getByRole("button", { name: "Browser" })).toBeInTheDocument();
		expect(within(group).getByRole("button", { name: "Device code" })).toBeInTheDocument();
		expect(within(group).getByRole("button", { name: "API key" })).toBeInTheDocument();
		expect(within(group).getByLabelText("Import JSON")).toBeInTheDocument();
		expect(within(group).getByRole("button", { name: "Close" })).toBeInTheDocument();
		await user.click(within(group).getByRole("button", { name: "Close" }));
		expect(screen.queryByRole("group", { name: "codex sign-in methods" })).toBeNull();
	});
	it("starts Codex device login, opens its page, and keeps its code visible", async () => {
		mock.post.mockResolvedValue({ data: { id: "device-1", provider: "codex", mode: "device", code: "ABCD-EFGH", url: "https://provider.test/device", status: "waiting", accountId: "" } });
		const user = userEvent.setup();
		renderAccounts();
		await screen.findByText(alice.email);
		await user.click(screen.getAllByRole("button", { name: "Add account" })[0]);
		await user.click(screen.getByRole("button", { name: "Device code" }));
		expect(mock.post).toHaveBeenCalledWith("/api/v1/provider-accounts/login", { body: { provider: "codex", mode: "device" } });
		const group = screen.getByRole("group", { name: "codex sign-in methods" });
		expect(await within(group).findByText(/ABCD-EFGH/)).toBeInTheDocument();
		await user.click(within(group).getByRole("button", { name: "Copy code" }));
		expect(mock.clipboard).toHaveBeenCalledWith("ABCD-EFGH");
		await user.click(within(group).getByRole("button", { name: "Copy link" }));
		expect(mock.clipboard).toHaveBeenCalledWith("https://provider.test/device");
		expect(mock.open).not.toHaveBeenCalled();
		await user.click(within(group).getByRole("button", { name: "Open sign-in" }));
		expect(mock.open).toHaveBeenCalledWith("https://provider.test/device");
	});
	it("sends the API key and base URL through AO without rendering the key", async () => {
		mock.post.mockResolvedValue({ data: { id: "key-1", provider: "codex", mode: "api_key", status: "waiting", accountId: "" } });
		const user = userEvent.setup();
		renderAccounts();
		await screen.findByText(alice.email);
		await user.click(screen.getAllByRole("button", { name: "Add account" })[0]);
		await user.click(screen.getByRole("button", { name: "API key" }));
		await user.type(screen.getByLabelText("API key"), "secret-api-key");
		await user.type(screen.getByRole("textbox", { name: "Base URL" }), "https://api.example.test");
		await user.click(screen.getByRole("button", { name: "Add API key" }));
		expect(mock.post).toHaveBeenCalledWith("/api/v1/provider-accounts/login", { body: { provider: "codex", mode: "api_key", apiKey: "secret-api-key", baseUrl: "https://api.example.test" } });
		expect(screen.queryByText("secret-api-key")).toBeNull();
	});
	it("reads a JSON file and sends its contents to the selected provider", async () => {
		mock.post.mockResolvedValue({ data: { id: "import-1", provider: "codex", mode: "import", status: "waiting", accountId: "" } });
		const user = userEvent.setup();
		renderAccounts();
		await screen.findByText(alice.email);
		await user.click(screen.getAllByRole("button", { name: "Add account" })[0]);
		const file = new File([JSON.stringify({ type: "codex", email: "imported@example.test", access_token: "secret" })], "codex.json", { type: "application/json" });
		await user.upload(within(screen.getByRole("group", { name: "codex sign-in methods" })).getByLabelText("Import JSON"), file);
		expect(mock.post).toHaveBeenCalledWith("/api/v1/provider-accounts/login", { body: { provider: "codex", mode: "import", credentialJson: JSON.stringify({ type: "codex", email: "imported@example.test", access_token: "secret" }) } });
	});
	it("does not offer device login for Claude", async () => {
		const user = userEvent.setup();
		renderAccounts();
		await screen.findByText(clara.email);
		await user.click(screen.getAllByRole("button", { name: "Add account" })[1]);
		const group = screen.getByRole("group", { name: "claude sign-in methods" });
		expect(within(group).queryByRole("button", { name: "Device code" })).toBeNull();
		expect(within(group).getByRole("button", { name: "Browser" })).toBeInTheDocument();
	});
});

describe("primary request switching", () => {
 it("can move existing Codex sessions at the next request boundary", async () => {
  const user = userEvent.setup();
  const cache = renderAccounts();
  const invalidate = vi.spyOn(cache, "invalidateQueries");
  await screen.findByText(bob.email);
  expect(screen.getByText(/choose whether it affects new sessions only/)).toBeInTheDocument();
  await user.click(within(accountRow(bob.email)).getByRole("button", { name: "Use as default" }));
	  await user.click(within(screen.getByRole("group", { name: "Confirm default change" })).getByRole("button", { name: "Move existing sessions" }));
  expect(mock.put).toHaveBeenCalledWith("/api/v1/provider-accounts/{accountId}/primary", { params: { path: { accountId: "b" } }, body: { moveExisting: true } });
  expect(await screen.findByRole("status")).toHaveTextContent("Default changed. Codex sessions using the previous default will use this account for their next API request.");
  expect(invalidate).toHaveBeenCalledWith({ queryKey: ["session-provider-account"] });
  expect(mock.post).not.toHaveBeenCalled();
  expect(mock.remove).not.toHaveBeenCalled();
 });
 it("can move existing Claude sessions at the next request boundary", async () => {
  inventory.accounts.push({ ...clara, id: "d", email: "claude-two@test.example", primary: false });
  const user = userEvent.setup();
  renderAccounts();
  await screen.findByText("claude-two@test.example");
  await user.click(within(accountRow("claude-two@test.example")).getByRole("button", { name: "Use as default" }));
	  const confirm = screen.getByRole("group", { name: "Confirm default change" });
	  await user.click(within(confirm).getByRole("button", { name: "Move existing sessions" }));
	  expect(mock.put).toHaveBeenCalledWith("/api/v1/provider-accounts/{accountId}/primary", { params: { path: { accountId: "d" } }, body: { moveExisting: true } });
	  expect(await screen.findByRole("status")).toHaveTextContent("Default changed. Claude sessions using the previous default will use this account for their next API request.");
 });
});
