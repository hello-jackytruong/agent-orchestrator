import { useQuery } from "@tanstack/react-query";
import type { components } from "../../api/schema";
import { apiClient, apiErrorMessage } from "../lib/api-client";

export type ProviderAccount = components["schemas"]["ProviderAccountView"];
export type ProviderAccounts = components["schemas"]["ProviderAccountsResponse"];
export type ProviderLogin = components["schemas"]["ProviderLoginResponse"];
export const providerAccountsKey = ["provider-accounts"] as const;
export function accountProvider(harness: string): string {
	return harness === "codex" ? "codex" : harness === "claude-code" ? "claude" : "";
}
export async function fetchProviderAccounts(): Promise<ProviderAccounts> {
	const { data, error } = await apiClient.GET("/api/v1/provider-accounts");
	if (error) throw new Error(apiErrorMessage(error));
	return data!;
}
export function useProviderAccounts(enabled = true) {
	return useQuery({ queryKey: providerAccountsKey, queryFn: fetchProviderAccounts, enabled, refetchInterval: 5000, retry: 1 });
}
export async function changeProviderAccount(accountId: string, action: "primary" | "sign-out" | "remove", replacementPrimaryId?: string): Promise<ProviderAccounts> {
	const params = { path: { accountId } };
	const body = { replacementPrimaryId };
	const result = action === "primary"
		? await apiClient.PUT("/api/v1/provider-accounts/{accountId}/primary", { params })
		: action === "sign-out"
			? await apiClient.POST("/api/v1/provider-accounts/{accountId}/sign-out", { params, body })
			: await apiClient.DELETE("/api/v1/provider-accounts/{accountId}", { params, body });
	if (result.error) throw new Error(apiErrorMessage(result.error));
	return result.data!;
}
export async function startProviderLogin(provider: "codex" | "claude", accountId?: string): Promise<ProviderLogin> {
	const { data, error } = await apiClient.POST("/api/v1/provider-accounts/login", { body: { provider, accountId } });
	if (error) throw new Error(apiErrorMessage(error));
	return data!;
}
export async function fetchProviderLogin(loginId: string): Promise<ProviderLogin> {
	const { data, error } = await apiClient.GET("/api/v1/provider-accounts/login/{loginId}", { params: { path: { loginId } } });
	if (error) throw Object.assign(new Error(apiErrorMessage(error)), { code: error.code });
	return data!;
}
export async function cancelProviderLogin(loginId: string): Promise<void> {
	const { error } = await apiClient.DELETE("/api/v1/provider-accounts/login/{loginId}", { params: { path: { loginId } } });
	if (error) throw new Error(apiErrorMessage(error));
}
