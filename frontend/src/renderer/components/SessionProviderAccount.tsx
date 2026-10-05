import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { apiClient, apiErrorMessage } from "../lib/api-client";
import { providerAccountsKey, useProviderAccounts } from "../hooks/useProviderAccounts";

export function SessionProviderAccount({ sessionId }: { sessionId: string }) {
	const { t } = useTranslation();
	const accounts = useProviderAccounts();
	const cache = useQueryClient();
	const [pending, setPending] = useState(false);
	const [message, setMessage] = useState<{ text: string; sessionId: string; accountId: string } | null>(null);
	const key = ["session-provider-account", sessionId];
	const route = useQuery({ queryKey: key, refetchInterval: 5000, queryFn: async () => {
		const { data, error } = await apiClient.GET("/api/v1/sessions/{sessionId}/provider-account", { params: { path: { sessionId } } });
		if (error) throw new Error(apiErrorMessage(error));
		return data!;
	} });
	useEffect(() => {
		if (message && (message.sessionId !== sessionId || message.accountId !== route.data?.accountId)) setMessage(null);
	}, [message, sessionId, route.data?.accountId]);
	if (!route.data?.managed) return null;
	async function switchAccount(accountId: string) {
		setPending(true); setMessage(null);
		try {
			const { data, error } = await apiClient.PUT("/api/v1/sessions/{sessionId}/provider-account", { params: { path: { sessionId } }, body: { accountId } });
			if (error) throw new Error(apiErrorMessage(error));
			cache.setQueryData(key, data);
			void cache.invalidateQueries({ queryKey: providerAccountsKey });
			setMessage({ text: t("providerAccounts.sessionChanged"), sessionId, accountId: data!.accountId });
		} catch (error) { setMessage({ text: error instanceof Error ? error.message : t("providerAccounts.sessionChangeFailed"), sessionId, accountId: route.data?.accountId ?? "" }); }
		finally { setPending(false); }
	}
	return <div className="flex flex-wrap items-center gap-2 px-3 py-1 text-xs border-b border-border">
		<label className="flex items-center gap-2">{t("providerAccounts.sessionLabel")}<select aria-label={t("providerAccounts.sessionLabel")} disabled={pending || !accounts.data} value={route.data.accountId} onChange={event => void switchAccount(event.target.value)}>
			{!route.data.accountId ? <option value="">{t("providerAccounts.loginRequired")}</option> : null}
			{accounts.data?.accounts.filter(a => a.provider === route.data?.provider && a.signedIn).map(a => <option key={a.id} value={a.id}>{a.email}{a.primary ? t("providerAccounts.primarySuffix") : ""}</option>)}
		</select></label><span className="text-muted-foreground">{t(route.data.provider === "codex" ? "providerAccounts.sessionNextRequest" : "providerAccounts.sessionIdle")}</span>
		{message?.sessionId === sessionId && message.accountId === route.data.accountId ? <span role="status">{message.text}</span> : null}
	</div>;
}
