import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import { cancelProviderLogin, changeProviderAccount, fetchProviderLogin, providerAccountsKey, startProviderLogin, useProviderAccounts, type ProviderAccount, type ProviderLogin } from "../../hooks/useProviderAccounts";
import { aoBridge } from "../../lib/bridge";
import { Button } from "../ui/button";
import { SettingsSection } from "./SettingsSection";

const loginKey = ["provider-account-login"] as const;

type Removal = { account: ProviderAccount; action: "remove" | "sign-out" };
export function ProviderAccountsSection({ titleHidden }: { titleHidden?: boolean }) {
	const { t } = useTranslation();
	const query = useProviderAccounts();
	const cache = useQueryClient();
	const [login, updateLogin] = useState<ProviderLogin | null>(() => cache.getQueryData<ProviderLogin>(loginKey) ?? null);
	function setLogin(next: ProviderLogin | null) {
		cache.setQueryData(loginKey, next);
		updateLogin(next);
	}
	const [pending, setPending] = useState(false);
	const [message, setMessage] = useState("");
	const [removal, setRemoval] = useState<Removal | null>(null);
	const [replacement, setReplacement] = useState("");
	const accounts = query.data?.accounts ?? [];
	useEffect(() => {
		if (!login || login.status !== "waiting") return;
		let mounted = true;
		let timer: ReturnType<typeof setTimeout>;
		const poll = async () => {
			try {
				const next = await fetchProviderLogin(login.id);
				if (!mounted) return;
				setLogin(next);
				if (next.status === "complete") {
					setMessage(t("providerAccounts.loginComplete"));
					void cache.invalidateQueries({ queryKey: providerAccountsKey });
				} else if (next.status === "failed") setMessage(t("providerAccounts.loginFailed"));
				else timer = setTimeout(poll, 1500);
			} catch (error) {
				if (!mounted) return;
				if (error instanceof Error && "code" in error && error.code === "PROVIDER_LOGIN_NOT_FOUND") {
					setLogin({ ...login, status: "failed" });
					setMessage(t("providerAccounts.loginFailed"));
				} else { setMessage(error instanceof Error ? error.message : t("providerAccounts.loginStatusFailed")); timer = setTimeout(poll, 3000); }
			}
		};
		timer = setTimeout(poll, 1000);
		return () => { mounted = false; clearTimeout(timer); };
	}, [login?.id, login?.status, cache, t]);
	async function run(action: () => Promise<void>) {
		setPending(true); setMessage("");
		try { await action(); } catch (error) { setMessage(error instanceof Error ? error.message : t("providerAccounts.operationFailed")); }
		finally { setPending(false); }
	}
	function beginLogin(provider: "codex" | "claude", accountId?: string) {
		void run(async () => {
			const next = await startProviderLogin(provider, accountId);
			setLogin(next);
			await aoBridge.app.openExternal(next.url);
		});
	}
	const alternatives = removal ? accounts.filter(a => a.provider === removal.account.provider && a.signedIn && a.id !== removal.account.id) : [];
	const needsReplacement = Boolean(removal?.account.primary && alternatives.length);
	return <SettingsSection title={t("providerAccounts.title")} sectionId="accountManager" titleHidden={titleHidden}>
		<p className="text-xs text-muted-foreground">{t("providerAccounts.instructions")}</p>
		{query.isLoading ? <p>{t("providerAccounts.loading")}</p> : null}
		{query.error ? <p role="alert">{query.error.message}</p> : null}
		{query.data?.recoveryRequired ? <p role="alert">{t("providerAccounts.recovery")}</p> : null}
		{(["codex", "claude"] as const).map(provider => <div key={provider} className="rounded-md bg-[var(--color-bg-settings-row)] p-3 space-y-2">
			<div className="flex items-center justify-between"><h3>{provider === "codex" ? "Codex" : "Claude"}</h3>
				<Button size="sm" variant="outline" disabled={pending || login?.status === "waiting"} onClick={() => beginLogin(provider)}>{t("settings.codexAccounts.add")}</Button></div>
			{accounts.filter(a => a.provider === provider).map(account => <div key={account.id} className="flex flex-wrap items-center gap-2 border-t border-border py-2">
				<div className="min-w-0 flex-1"><div className="truncate">{account.email}</div><div className="text-xs text-muted-foreground">{t("providerAccounts.sessionSummary", { status: account.signedIn ? account.primary ? t("providerAccounts.primary") : t("settings.codexAccounts.signedIn") : t("settings.codexAccounts.signedOut"), count: account.sessions.length })}</div></div>
				{account.signedIn ? <>
					{!account.primary ? <Button size="sm" variant="ghost" disabled={pending} onClick={() => void run(async () => { cache.setQueryData(providerAccountsKey, await changeProviderAccount(account.id, "primary")); setMessage(t("providerAccounts.primaryChanged")); })}>{t("providerAccounts.makePrimary")}</Button> : null}
					<Button size="sm" variant="ghost" disabled={pending} onClick={() => { setReplacement(""); setRemoval({ account, action: "sign-out" }); }}>{t("shell.signOut")}</Button>
				</> : <Button size="sm" variant="ghost" disabled={pending || login?.status === "waiting"} onClick={() => beginLogin(provider, account.id)}>{t("settings.codexAccounts.signInAgain")}</Button>}
				<Button size="sm" variant="ghost" disabled={pending} onClick={() => { setReplacement(""); setRemoval({ account, action: "remove" }); }}>{t("shell.remove")}</Button>
			</div>)}
			{query.data && !accounts.some(a => a.provider === provider && a.signedIn) ? <p className="text-xs text-muted-foreground">{t("providerAccounts.emptyAccounts", { provider: provider === "codex" ? "Codex" : "Claude" })} {query.data?.defaults.find(d => d.provider === provider)?.managed ? t("providerAccounts.managedNeedsLogin") : t("providerAccounts.deviceSessionsContinue")}</p> : null}
		</div>)}
		{login?.status === "waiting" ? <div className="flex flex-wrap items-center gap-2"><p>{t("providerAccounts.browserSignIn")}</p><Button size="sm" variant="outline" onClick={() => void run(async () => { await aoBridge.app.openExternal(login.url); })}>{t("providerAccounts.openSignIn")}</Button><Button size="sm" variant="ghost" disabled={pending} onClick={() => void run(async () => { await cancelProviderLogin(login.id); setLogin(null); })}>{t("confirm.cancel")}</Button></div> : null}
		{removal ? <div role="group" aria-label={t("providerAccounts.confirmChange")} className="rounded-md border border-border p-3 space-y-2">
			<p>{t(removal.action === "sign-out" ? "providerAccounts.confirmSignOut" : "providerAccounts.confirmRemove", { email: removal.account.email })}</p>
			<p className="text-xs text-muted-foreground">{removal.account.sessions.length ? t("providerAccounts.affectedSessions") : t("providerAccounts.noSessions")} {!accounts.some(a => a.provider === removal.account.provider && a.signedIn && a.id !== removal.account.id) ? t("providerAccounts.noAccountsConsequence") : ""}</p>
			{needsReplacement ? <label className="flex items-center gap-2">{t("providerAccounts.newPrimary")}<select aria-label={t("providerAccounts.replacementPrimary")} value={replacement} onChange={event => setReplacement(event.target.value)}><option value="">{t("providerAccounts.chooseAccount")}</option>{alternatives.map(a => <option key={a.id} value={a.id}>{a.email}</option>)}</select></label> : null}
			<div className="flex gap-2"><Button size="sm" disabled={pending || (needsReplacement && !replacement)} onClick={() => void run(async () => { cache.setQueryData(providerAccountsKey, await changeProviderAccount(removal.account.id, removal.action, replacement || undefined)); setRemoval(null); setMessage(t("providerAccounts.operationComplete")); })}>{t("confirm.confirm")}</Button><Button size="sm" variant="ghost" disabled={pending} onClick={() => setRemoval(null)}>{t("confirm.cancel")}</Button></div>
		</div> : null}
		{message ? <p role="status" className="text-xs">{message}</p> : null}
	</SettingsSection>;
}
