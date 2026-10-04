import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { SessionProviderAccount } from "./SessionProviderAccount";
import type { components } from "../../api/schema";
const mock = vi.hoisted(() => ({ get: vi.fn(), put: vi.fn() }));
vi.mock("../lib/api-client", () => ({ apiClient: { GET: mock.get, PUT: mock.put }, apiErrorMessage: (error: { message: string }) => error.message }));
let route: components["schemas"]["SessionProviderAccountResponse"];
let inventory: components["schemas"]["ProviderAccountsResponse"];
function renderSession(id = "session-one") {
	const cache = new QueryClient({ defaultOptions: { queries: { retry: false } } });
	render(<QueryClientProvider client={cache}><SessionProviderAccount sessionId={id} /></QueryClientProvider>);
	return cache;
}
beforeEach(() => {
	vi.clearAllMocks();
	route = { managed: true, provider: "codex", accountId: "a", loginRequired: false };
	inventory = { accounts: [
		{ id: "a", provider: "codex", email: "alice@test.example", signedIn: true, primary: true, sessions: ["session-one"] },
		{ id: "b", provider: "codex", email: "bob@test.example", signedIn: true, primary: false, sessions: [] },
		{ id: "c", provider: "claude", email: "clara@test.example", signedIn: true, primary: true, sessions: [] },
		{ id: "d", provider: "codex", email: "signed-out@test.example", signedIn: false, primary: false, sessions: [] },
	], defaults: [{ provider: "codex", primaryId: "a", managed: true }, { provider: "claude", primaryId: "c", managed: true }], recoveryRequired: false };
	mock.get.mockImplementation(async (path: string) => ({ data: path === "/api/v1/provider-accounts" ? inventory : route }));
	mock.put.mockImplementation(async (_path: string, input: { body: { accountId: string } }) => ({ data: { ...route, accountId: input.body.accountId } }));
});
afterEach(cleanup);
describe("session provider account", () => {
	it("shows the assigned account rather than changing to the current primary", async () => {
		route.accountId = "b";
		renderSession();
		expect(await screen.findByRole("combobox", { name: "Session account" })).toHaveValue("b");
		expect(mock.put).not.toHaveBeenCalled();
	});
	it("offers only signed-in accounts for the session provider", async () => {
		renderSession();
		const select = await screen.findByRole("combobox", { name: "Session account" });
		await waitFor(() => expect(select.querySelectorAll("option")).toHaveLength(2));
		expect([...select.querySelectorAll("option")].map(o => o.textContent)).toEqual(["alice@test.example (primary)", "bob@test.example"]);
		expect(screen.queryByRole("option", { name: "clara@test.example" })).toBeNull();
		expect(screen.queryByRole("option", { name: "signed-out@test.example" })).toBeNull();
	});
	it("sends the selected account and owning session without changing the primary", async () => {
		const user = userEvent.setup();
		renderSession();
		await user.selectOptions(await screen.findByRole("combobox", { name: "Session account" }), "b");
		expect(mock.put).toHaveBeenCalledWith("/api/v1/sessions/{sessionId}/provider-account", { params: { path: { sessionId: "session-one" } }, body: { accountId: "b" } });
		expect(await screen.findByRole("status")).toHaveTextContent("Session account changed.");
		expect(screen.getByRole("combobox", { name: "Session account" })).toHaveValue("b");
		expect(mock.put).toHaveBeenCalledTimes(1);
	});
	it("retains the existing account and explains a busy-session refusal", async () => {
		mock.put.mockResolvedValue({ error: { message: "Wait until this session is idle" } });
		const user = userEvent.setup(); renderSession();
		await user.selectOptions(await screen.findByRole("combobox", { name: "Session account" }), "b");
		expect(await screen.findByRole("status")).toHaveTextContent("Wait until this session is idle");
		expect(screen.getByRole("combobox", { name: "Session account" })).toHaveValue("a");
		expect(screen.getByRole("combobox", { name: "Session account" })).toBeEnabled();
	});
	it("drops an earlier busy warning when account removal reroutes the session", async () => {
		mock.put.mockResolvedValue({ error: { message: "Wait until this session is idle" } });
		const user = userEvent.setup();
		const cache = renderSession();
		await user.selectOptions(await screen.findByRole("combobox", { name: "Session account" }), "b");
		expect(await screen.findByRole("status")).toHaveTextContent("Wait until this session is idle");
		cache.setQueryData(["session-provider-account", "session-one"], { ...route, accountId: "b" });
		await waitFor(() => expect(screen.getByRole("combobox", { name: "Session account" })).toHaveValue("b"));
		expect(screen.queryByRole("status")).toBeNull();
		cache.setQueryData(["session-provider-account", "session-one"], { ...route, accountId: "a" });
		await waitFor(() => expect(screen.getByRole("combobox", { name: "Session account" })).toHaveValue("a"));
		expect(screen.queryByRole("status")).toBeNull();
		expect(mock.put).toHaveBeenCalledTimes(1);
	});
	it("shows login required without an obsolete busy warning after last-account removal", async () => {
		mock.put.mockResolvedValue({ error: { message: "Wait until this session is idle" } });
		const user = userEvent.setup();
		const cache = renderSession();
		await user.selectOptions(await screen.findByRole("combobox", { name: "Session account" }), "b");
		expect(await screen.findByRole("status")).toHaveTextContent("Wait until this session is idle");
		cache.setQueryData(["provider-accounts"], { ...inventory, accounts: inventory.accounts.filter(a => a.provider !== "codex") });
		cache.setQueryData(["session-provider-account", "session-one"], { ...route, accountId: "", loginRequired: true });
		await waitFor(() => expect(screen.getByRole("combobox", { name: "Session account" })).toHaveValue(""));
		expect(screen.getByRole("option", { name: "Please sign in again" })).toBeInTheDocument();
		expect(screen.queryByRole("status")).toBeNull();
		expect(screen.queryByRole("option", { name: "bob@test.example" })).toBeNull();
		expect(mock.put).toHaveBeenCalledTimes(1);
	});
	it("does not retain a success notice after another operation changes the route", async () => {
		const user = userEvent.setup();
		const cache = renderSession();
		await user.selectOptions(await screen.findByRole("combobox", { name: "Session account" }), "b");
		expect(await screen.findByRole("status")).toHaveTextContent("Session account changed.");
		cache.setQueryData(["session-provider-account", "session-one"], { ...route, accountId: "a" });
		await waitFor(() => expect(screen.getByRole("combobox", { name: "Session account" })).toHaveValue("a"));
		expect(screen.queryByRole("status")).toBeNull();
		expect(mock.put).toHaveBeenCalledTimes(1);
	});
	it("does not attach a late refusal to a route changed by account management", async () => {
		let complete: ((value: unknown) => void) | undefined;
		mock.put.mockImplementation(() => new Promise(resolve => { complete = resolve; }));
		const user = userEvent.setup();
		const cache = renderSession();
		const select = await screen.findByRole("combobox", { name: "Session account" });
		await user.selectOptions(select, "b");
		expect(select).toBeDisabled();
		cache.setQueryData(["session-provider-account", "session-one"], { ...route, accountId: "b" });
		await waitFor(() => expect(select).toHaveValue("b"));
		complete!({ error: { message: "Wait until this session is idle" } });
		await waitFor(() => expect(select).toBeEnabled());
		expect(screen.queryByRole("status")).toBeNull();
		expect(select).toHaveValue("b");
		expect(mock.put).toHaveBeenCalledTimes(1);
	});
	it("retains the current account when the helper is unavailable", async () => {
		mock.put.mockRejectedValue(new Error("Account helper unavailable"));
		const user = userEvent.setup(); renderSession();
		await user.selectOptions(await screen.findByRole("combobox", { name: "Session account" }), "b");
		expect(await screen.findByRole("status")).toHaveTextContent("Account helper unavailable");
		expect(screen.getByRole("combobox", { name: "Session account" })).toHaveValue("a");
	});
	it("does not offer managed account switching for older native sessions", async () => {
		route.managed = false; route.accountId = ""; route.provider = "";
		renderSession();
		await waitFor(() => expect(mock.get).toHaveBeenCalledWith("/api/v1/sessions/{sessionId}/provider-account", { params: { path: { sessionId: "session-one" } } }));
		expect(screen.queryByRole("combobox", { name: "Session account" })).toBeNull();
		expect(mock.put).not.toHaveBeenCalled();
	});
	it("shows a login-required session and allows selecting an available account", async () => {
		route.accountId = ""; route.loginRequired = true;
		const user = userEvent.setup(); renderSession();
		const select = await screen.findByRole("combobox", { name: "Session account" });
		expect(screen.getByRole("option", { name: "Please sign in again" })).toBeInTheDocument();
		expect(select).toHaveValue("");
		await user.selectOptions(select, "a");
		expect(mock.put).toHaveBeenCalledWith("/api/v1/sessions/{sessionId}/provider-account", { params: { path: { sessionId: "session-one" } }, body: { accountId: "a" } });
	});
	it("disables selection while a change is awaiting acknowledgement", async () => {
		let complete: ((value: unknown) => void) | undefined;
		mock.put.mockImplementation(() => new Promise(resolve => { complete = resolve; }));
		const user = userEvent.setup(); renderSession();
		const select = await screen.findByRole("combobox", { name: "Session account" });
		await user.selectOptions(select, "b");
		expect(select).toBeDisabled();
		complete!({ data: { ...route, accountId: "b" } });
		await waitFor(() => expect(select).toBeEnabled());
		expect(select).toHaveValue("b");
	});
});
