import { useEffect, useRef, useState } from "react";
import type { ChangeEvent } from "react";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import { useQueryClient } from "@tanstack/react-query";
import { AlertCircle, Check, ChevronDown, Copy, FileUp, KeyRound, LogIn, LogOut, MonitorSmartphone, Plus, ShieldCheck, Trash2, UserRound, X } from "lucide-react";
import { cancelProviderLogin, changeProviderAccount, fetchProviderLogin, providerAccountsKey, setQuotaAutoSwitch, startProviderLogin, useProviderAccounts, type ProviderAccount, type ProviderLogin } from "../../hooks/useProviderAccounts";
import { aoBridge } from "../../lib/bridge";
import { Button } from "../ui/button";
import { Badge } from "../ui/badge";
import { Card } from "../ui/card";
import { SettingsSection } from "./SettingsSection";

const loginKey = ["provider-account-login"] as const;

type UsageRow = { label: string; text: string; percent: number | null };

function usageRows(account: ProviderAccount, translate: TFunction): UsageRow[] {
	const usage = account.usage;
	if (!usage) return [];
	if (usage.status !== "available") return [{ label: "Usage", text: translate("providerAccounts.usageUnavailable"), percent: null }];
	const windows = usage.windows ?? [];
	if (!windows.length) return [{ label: usage.plan || "Usage", text: usage.plan || translate("providerAccounts.usageUnavailable"), percent: null }];
	return windows.map((window, index) => {
		const parts = [index === 0 ? usage.plan : ""];
		parts.push(translate("providerAccounts.usageRemaining", { percent: Math.round(window.remainingFraction * 100) }));
		if (window.resetTime) parts.push(translate("providerAccounts.usageResets", { reset: formatResetTime(window.resetTime) }));
		return { label: window.name || (index === 0 ? usage.plan || "Usage" : "Usage"), text: parts.filter(Boolean).join(" · "), percent: usagePercent(window.remainingFraction) };
	});
}

function formatResetTime(value: string): string {
	const date = new Date(value);
	if (Number.isNaN(date.getTime())) return value;
	return new Intl.DateTimeFormat(undefined, {
		month: "short",
		day: "numeric",
		hour: "numeric",
		minute: "2-digit",
	}).format(date);
}

function usagePercent(value: number): number | null {
	return typeof value === "number" && Number.isFinite(value) ? Math.max(0, Math.min(100, Math.round(value * 100))) : null;
}

type Removal = { account: ProviderAccount; action: "remove" | "sign-out" };
type PrimaryChange = { account: ProviderAccount };
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
	const [primaryChange, setPrimaryChange] = useState<PrimaryChange | null>(null);
	const [replacement, setReplacement] = useState("");
	const [choiceProvider, setChoiceProvider] = useState<"codex" | "claude" | null>(() => {
		const cached = cache.getQueryData<ProviderLogin>(loginKey);
		return cached?.status === "waiting" && (cached.provider === "codex" || cached.provider === "claude") ? cached.provider : null;
	});
	const [collapsedProviders, setCollapsedProviders] = useState<Record<"codex" | "claude", boolean>>({ codex: false, claude: false });
	const [apiKeyOpen, setApiKeyOpen] = useState(false);
	const [apiKey, setApiKey] = useState("");
	const [baseUrl, setBaseUrl] = useState("");
	const [label, setLabel] = useState("");
	const [copiedLoginValue, setCopiedLoginValue] = useState<"link" | "code" | null>(null);
	const loginStatusFailures = useRef(0);
	const accounts = query.data?.accounts ?? [];
	useEffect(() => {
		if (!login || login.status !== "waiting") return;
		let mounted = true;
		let timer: ReturnType<typeof setTimeout>;
		loginStatusFailures.current = 0;
		const poll = async () => {
			try {
				const next = await fetchProviderLogin(login.id);
				if (!mounted) return;
				setLogin(next);
				if (next.status === "complete") {
					setMessage(t("providerAccounts.loginComplete"));
					setChoiceProvider(null);
					void cache.invalidateQueries({ queryKey: providerAccountsKey });
				} else if (next.status === "failed") {
					setChoiceProvider(null);
					setMessage(t("providerAccounts.loginFailed"));
				}
				else timer = setTimeout(poll, 1500);
			} catch (error) {
				if (!mounted) return;
				if (error instanceof Error && "code" in error && error.code === "PROVIDER_LOGIN_NOT_FOUND") {
					setLogin({ ...login, status: "failed" });
					setChoiceProvider(null);
					setMessage(t("providerAccounts.loginFailed"));
				} else if (++loginStatusFailures.current >= 5) {
					setLogin({ ...login, status: "failed" });
					setChoiceProvider(null);
					setMessage(t("providerAccounts.loginStatusFailed"));
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
	async function copyLoginValue(value: string, kind: "link" | "code") {
		try {
			await aoBridge.clipboard.writeText(value);
			setCopiedLoginValue(kind);
		} catch (error) {
			setMessage(error instanceof Error ? error.message : t("providerAccounts.operationFailed"));
		}
	}
	function beginLogin(provider: "codex" | "claude", accountId?: string, mode: "browser" | "device" | "import" | "api_key" = "browser", credentialJson?: string) {
		void run(async () => {
			setCopiedLoginValue(null);
			const next = await startProviderLogin({ provider, accountId, mode, apiKey: mode === "api_key" ? apiKey : undefined, baseUrl: mode === "api_key" ? baseUrl : undefined, label: mode === "api_key" ? label : undefined, credentialJson });
			setLogin(next);
			setChoiceProvider(provider);
			setApiKeyOpen(false);
			setApiKey(""); setBaseUrl(""); setLabel("");
		});
	}
	function chooseImport(provider: "codex" | "claude", accountId?: string) {
		return async (event: ChangeEvent<HTMLInputElement>) => {
			const file = event.target.files?.[0];
			if (!file) return;
			try {
				if (file.size > 1 << 20) throw new Error("Credential JSON exceeds 1 MiB.");
				beginLogin(provider, accountId, "import", await file.text());
			} catch (error) { setMessage(error instanceof Error ? error.message : t("providerAccounts.operationFailed")); }
			event.target.value = "";
		};
	}
	const alternatives = removal ? accounts.filter(a => a.provider === removal.account.provider && a.signedIn && a.id !== removal.account.id) : [];
	const needsReplacement = Boolean(removal?.account.primary && alternatives.length);
	function removalCardFor(accountId: string) {
		if (removal?.account.id !== accountId) return null;
		return <Card role="group" aria-label={t("providerAccounts.confirmChange")} className="border-warning/40 bg-warning/[0.04] shadow-none"><div className="flex gap-3 p-4"><div className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-warning/10 text-warning"><AlertCircle aria-hidden="true" className="size-4" /></div><div className="min-w-0 flex-1"><p className="font-medium">{t(removal.action === "sign-out" ? "providerAccounts.confirmSignOut" : "providerAccounts.confirmRemove", { email: removal.account.email })}</p><p className="mt-1 text-xs leading-5 text-muted-foreground">{removal.account.sessions.length ? t("providerAccounts.affectedSessions") : t("providerAccounts.noSessions")} {!accounts.some(a => a.provider === removal.account.provider && a.signedIn && a.id !== removal.account.id) ? ` ${t("providerAccounts.noAccountsConsequence")}` : ""}</p>{needsReplacement ? <label className="mt-3 flex flex-wrap items-center gap-2 text-xs font-medium">{t("providerAccounts.newPrimary")}<select className="h-8 rounded-md border border-border bg-background px-2 text-xs font-normal" aria-label={t("providerAccounts.replacementPrimary")} value={replacement} onChange={event => setReplacement(event.target.value)}><option value="">{t("providerAccounts.chooseAccount")}</option>{alternatives.map(a => <option key={a.id} value={a.id}>{a.email}</option>)}</select></label> : null}<div className="mt-4 flex flex-wrap gap-2"><Button size="sm" disabled={pending || (needsReplacement && !replacement)} onClick={() => void run(async () => { cache.setQueryData(providerAccountsKey, await changeProviderAccount(removal.account.id, removal.action, replacement || undefined)); setRemoval(null); setMessage(t("providerAccounts.operationComplete")); })}>{t("confirm.confirm")}</Button><Button size="sm" variant="ghost" disabled={pending} onClick={() => setRemoval(null)}>{t("confirm.cancel")}</Button></div></div></div></Card>;
	}
	function primaryChangeCardFor(account: ProviderAccount) {
		if (primaryChange?.account.id !== account.id) return null;
		const apply = (moveExisting: boolean) => void run(async () => {
			cache.setQueryData(providerAccountsKey, await changeProviderAccount(account.id, "primary", undefined, moveExisting));
			void cache.invalidateQueries({ queryKey: ["session-provider-account"] });
			setPrimaryChange(null);
			setMessage(t(moveExisting ? "providerAccounts.primaryRequestsChanged" : "providerAccounts.primaryChanged", { provider: account.provider === "codex" ? "Codex" : "Claude" }));
		});
		return <Card role="group" aria-label={t("providerAccounts.confirmDefaultChange")} className="border-primary/30 bg-primary/[0.04] shadow-none"><div className="flex gap-3 p-4"><div className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary"><Check aria-hidden="true" className="size-4" /></div><div className="min-w-0 flex-1"><p className="font-medium">{t("providerAccounts.confirmDefault", { email: account.email })}</p><p className="mt-1 text-xs leading-5 text-muted-foreground">{t("providerAccounts.defaultScope")}</p><div className="mt-4 flex flex-wrap gap-2"><Button size="sm" disabled={pending} onClick={() => apply(false)}>{t("providerAccounts.newSessionsOnly")}</Button><Button size="sm" variant="secondary" disabled={pending} onClick={() => apply(true)}>{t("providerAccounts.moveExistingSessions")}</Button><Button size="sm" variant="ghost" disabled={pending} onClick={() => setPrimaryChange(null)}>{t("confirm.cancel")}</Button></div></div></div></Card>;
	}
	return <SettingsSection title={t("providerAccounts.title")} sectionId="accountManager" titleHidden={titleHidden}>
		<div className="flex flex-col gap-4">
			<div className="rounded-xl border border-border/70 bg-card/50 p-4 shadow-sm">
				<div className="flex items-start gap-3">
					<div className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary"><ShieldCheck aria-hidden="true" className="size-5" /></div>
					<div className="min-w-0">
						<h3 className="font-heading text-sm font-semibold">{t("providerAccounts.title")}</h3>
					<p className="mt-1 max-w-2xl text-xs leading-5 text-muted-foreground">{t("providerAccounts.instructions")}</p>
					</div>
				</div>
				{query.isLoading ? <p className="mt-3 text-xs text-muted-foreground">{t("providerAccounts.loading")}</p> : null}
				{query.error ? <p role="alert" className="mt-3 flex items-center gap-2 text-xs text-destructive"><AlertCircle aria-hidden="true" className="size-3.5" />{query.error.message}</p> : null}
				{query.data?.recoveryRequired ? <p role="alert" className="mt-3 flex items-center gap-2 text-xs text-warning"><AlertCircle aria-hidden="true" className="size-3.5" />{t("providerAccounts.recovery")}</p> : null}
			</div>

			{(["codex", "claude"] as const).map(provider => {
				const providerAccounts = accounts.filter(account => account.provider === provider);
				const signedInCount = providerAccounts.filter(account => account.signedIn).length;
				const providerName = provider === "codex" ? "Codex" : "Claude";
				const isCollapsed = collapsedProviders[provider];
				return <div key={provider} data-testid={`provider-section-${provider}`} className="overflow-hidden rounded-xl border border-border/70 bg-card/35 shadow-sm">
					<div className="flex items-center justify-between gap-4 border-b border-border/60 px-4 py-4">
						<button type="button" className="flex min-w-0 flex-1 items-center gap-3 rounded-lg p-1 text-left transition-colors hover:bg-muted/40" aria-expanded={!isCollapsed} aria-controls={`provider-content-${provider}`} onClick={() => setCollapsedProviders(current => ({ ...current, [provider]: !current[provider] }))}>
							<div className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-muted text-foreground"><UserRound aria-hidden="true" className="size-4" /></div>
							<div className="min-w-0 flex-1"><h3 className="font-heading text-sm font-semibold">{providerName}</h3><p className="mt-0.5 text-xs text-muted-foreground">{signedInCount} signed in · {providerAccounts.length} accounts</p></div>
							<ChevronDown aria-hidden="true" className={`size-4 shrink-0 text-muted-foreground transition-transform ${isCollapsed ? "-rotate-90" : ""}`} />
						</button>
						<Button size="icon" variant="outline" className="shrink-0" aria-label={t("settings.codexAccounts.add")} title={t("settings.codexAccounts.add")} disabled={pending || login?.status === "waiting"} onClick={() => { const next = choiceProvider === provider ? null : provider; setChoiceProvider(next); setApiKeyOpen(false); }}><Plus aria-hidden="true" className="size-4" /></Button>
					</div>

					<div id={`provider-content-${provider}`} hidden={isCollapsed}>
					<div className="mx-4 mt-4 flex items-start gap-3 rounded-lg border border-border/60 bg-muted/20 px-3 py-3">
						<input className="mt-0.5 size-4 accent-primary" type="checkbox" checked={Boolean(provider === "codex" ? query.data?.codexQuotaAutoSwitch : query.data?.claudeQuotaAutoSwitch)} disabled={pending || query.isLoading || signedInCount < 2} aria-label={t("providerAccounts.quotaAutoSwitch", { provider: providerName })} aria-describedby={signedInCount < 2 ? `provider-accounts-${provider}-quota-auto-switch-info` : undefined} onChange={event => void run(async () => { cache.setQueryData(providerAccountsKey, await setQuotaAutoSwitch(provider, event.target.checked)); })} />
						<div className="min-w-0 flex-1"><p className="text-xs font-medium text-foreground">{t("providerAccounts.quotaAutoSwitch", { provider: providerName })}</p>{signedInCount < 2 ? <p id={`provider-accounts-${provider}-quota-auto-switch-info`} className="mt-1 flex items-center gap-1.5 text-2xs text-muted-foreground"><AlertCircle role="img" aria-label={t("providerAccounts.quotaAutoSwitchRequiresAccount", { provider: providerName })} className="size-3" />{t("providerAccounts.quotaAutoSwitchRequiresAccount", { provider: providerName })}</p> : null}</div>
					</div>

					{choiceProvider === provider ? <div role="group" aria-label={`${provider} sign-in methods`} className="mx-4 mb-4 mt-3 rounded-lg border border-primary/25 bg-primary/[0.04] p-3">
						<div className="mb-3 flex items-start gap-2"><KeyRound aria-hidden="true" className="mt-0.5 size-4 text-primary" /><div className="min-w-0 flex-1"><p className="text-xs font-medium">Add a {providerName} account</p><p className="mt-0.5 text-2xs text-muted-foreground">Choose the sign-in method that matches your account.</p></div><Button size="icon" variant="ghost" className="-mr-1 -mt-1 size-7 shrink-0" aria-label={t("common.close")} title={t("common.close")} disabled={login?.status === "waiting"} onClick={() => { setChoiceProvider(null); setApiKeyOpen(false); }}><X aria-hidden="true" className="size-4" /></Button></div>
						<div className="flex flex-wrap gap-2">
							<Button size="sm" variant="secondary" className="gap-1.5" disabled={pending || login?.status === "waiting"} onClick={() => beginLogin(provider)}><LogIn aria-hidden="true" className="size-3.5" />{t("settings.browserProfiles")}</Button>
							{provider === "codex" ? <Button size="sm" variant="secondary" className="gap-1.5" disabled={pending || login?.status === "waiting"} onClick={() => beginLogin(provider, undefined, "device")}><MonitorSmartphone aria-hidden="true" className="size-3.5" />{t("providerAccounts.deviceMethod")}</Button> : null}
							<Button size="sm" variant={apiKeyOpen ? "primary" : "secondary"} className="gap-1.5" disabled={pending || login?.status === "waiting"} onClick={() => { setApiKeyOpen(true); setApiKey(""); setBaseUrl(""); setLabel(""); }}><KeyRound aria-hidden="true" className="size-3.5" />{t("providerAccounts.apiKeyLabel")}</Button>
							<label className="inline-flex cursor-pointer items-center gap-1.5 rounded-md border border-border bg-background px-2.5 py-1.5 text-xs font-medium hover:bg-muted"><FileUp aria-hidden="true" className="size-3.5" />{t("providerAccounts.importMethod")}<input className="sr-only" aria-label={t("providerAccounts.importMethod")} type="file" disabled={pending || login?.status === "waiting"} accept=".json,application/json" onChange={event => void chooseImport(provider)(event)} /></label>
						</div>
						{login?.status === "waiting" && login.provider === provider ? <div className="mt-3 flex flex-wrap items-center gap-3 rounded-md border border-primary/25 bg-background/50 p-3">
							<div className="flex size-8 shrink-0 items-center justify-center rounded-md bg-primary/10 text-primary"><MonitorSmartphone aria-hidden="true" className="size-4" /></div>
							<div className="min-w-0 flex-1"><p className="text-xs font-medium">{login.mode === "device" ? `${t("providerAccounts.deviceSignIn")} ${login.code ?? ""}` : login.mode === "import" ? t("providerAccounts.importWaiting") : login.mode === "api_key" ? t("providerAccounts.apiKeyWaiting") : t("providerAccounts.browserSignIn")}</p><p className="mt-1 text-2xs text-muted-foreground">Your account manager will update when the sign-in finishes.</p></div>
							<div className="flex flex-wrap gap-2">{login.url ? <><Button size="sm" variant="outline" className="gap-1.5" onClick={() => void run(async () => { await aoBridge.app.openExternal(login.url!); })}><LogIn aria-hidden="true" className="size-3.5" />{t("providerAccounts.openSignIn")}</Button><Button size="sm" variant="outline" className="gap-1.5" onClick={() => void copyLoginValue(login.url!, "link")}><Copy aria-hidden="true" className="size-3.5" />{copiedLoginValue === "link" ? t("startup.commandCopied") : t("link.copy")}</Button></> : null}{login.code ? <Button size="sm" variant="outline" className="gap-1.5" onClick={() => void copyLoginValue(login.code!, "code")}><Copy aria-hidden="true" className="size-3.5" />{copiedLoginValue === "code" ? t("startup.commandCopied") : t("providerAccounts.copyCode")}</Button> : null}<Button size="sm" variant="ghost" disabled={pending} onClick={() => void run(async () => { await cancelProviderLogin(login.id); setLogin(null); setCopiedLoginValue(null); })}>{t("confirm.cancel")}</Button></div>
						</div> : null}
						{apiKeyOpen ? <div className="mt-3 grid gap-2 sm:grid-cols-[1fr_1fr]">
							<input className="h-8 rounded-md border border-border bg-background px-2.5 text-xs outline-none focus-visible:ring-2 focus-visible:ring-ring" aria-label={t("providerAccounts.apiKeyLabel")} value={apiKey} onChange={event => setApiKey(event.target.value)} placeholder={t("providerAccounts.apiKeyPlaceholder")} type="password" />
							<input className="h-8 rounded-md border border-border bg-background px-2.5 text-xs outline-none focus-visible:ring-2 focus-visible:ring-ring" aria-label={t("providerAccounts.baseUrlLabel")} value={baseUrl} onChange={event => setBaseUrl(event.target.value)} placeholder="https://api.example.com" />
							<input className="h-8 rounded-md border border-border bg-background px-2.5 text-xs outline-none focus-visible:ring-2 focus-visible:ring-ring sm:col-span-2" aria-label={t("providerAccounts.displayLabel")} value={label} onChange={event => setLabel(event.target.value)} placeholder={t("providerAccounts.displayLabel")} />
							<Button size="sm" className="w-fit gap-1.5 sm:col-span-2" disabled={pending || !apiKey.trim() || !baseUrl.trim()} onClick={() => beginLogin(provider, undefined, "api_key")}><Check aria-hidden="true" className="size-3.5" />{t("providerAccounts.addApiKey")}</Button>
						</div> : null}
					</div> : null}

					<div className="grid gap-3 p-4">
						{providerAccounts.map(account => {
							const accountUsage = usageRows(account, t);
							const status = account.signedIn ? account.primary ? t("providerAccounts.default") : t("settings.codexAccounts.signedIn") : t("settings.codexAccounts.signedOut");
							return <div key={account.id} className="space-y-2"><Card data-testid={`provider-account-${account.id}`} className={`gap-0 border shadow-none ${account.primary ? "border-primary/60 bg-primary/[0.10] ring-1 ring-primary/20" : "border-border/70 bg-background/35"}`}>
								<div className="flex flex-wrap items-start gap-3 p-4">
									<div className={`flex size-9 shrink-0 items-center justify-center rounded-lg ${account.signedIn ? "bg-success/10 text-success" : "bg-muted text-muted-foreground"}`}><ShieldCheck aria-hidden="true" className="size-4" /></div>
									<div className="min-w-0 flex-1"><div className="flex flex-wrap items-center gap-2"><div className="truncate font-medium">{account.email}</div>{account.primary ? <Badge variant="accent" className="h-5 gap-1 px-1.5 text-2xs"><Check aria-hidden="true" className="size-3" />{t("providerAccounts.default")}</Badge> : null}</div><div className="mt-1 text-xs text-muted-foreground">{t("providerAccounts.sessionSummary", { status, count: account.sessions.length })}</div></div>
									<div className="flex shrink-0 flex-wrap items-center justify-end gap-1">
										{account.signedIn ? <>{!account.primary ? <Button size="sm" variant="ghost" className="h-7 gap-1 px-2 text-xs" disabled={pending} onClick={() => { setRemoval(null); setPrimaryChange({ account }); }}><Check aria-hidden="true" className="size-3.5" />{t("providerAccounts.useAsDefault")}</Button> : null}<Button size="sm" variant="ghost" className="h-7 gap-1 px-2 text-xs" disabled={pending} onClick={() => { setPrimaryChange(null); setReplacement(""); setRemoval({ account, action: "sign-out" }); }}><LogOut aria-hidden="true" className="size-3.5" />{t("shell.signOut")}</Button></> : <><Button size="sm" variant="ghost" className="h-7 gap-1 px-2 text-xs" disabled={pending || login?.status === "waiting"} onClick={() => beginLogin(provider, account.id)}><LogIn aria-hidden="true" className="size-3.5" />{t("settings.codexAccounts.signInAgain")}</Button><Button size="icon" variant="ghost" className="text-destructive hover:text-destructive" aria-label={t("shell.remove")} title={t("shell.remove")} disabled={pending} onClick={() => { setPrimaryChange(null); setReplacement(""); setRemoval({ account, action: "remove" }); }}><Trash2 aria-hidden="true" className="size-4" /></Button></>}
									</div>
								</div>
								<div className="border-t border-border/60 px-4 py-3">
									{account.signedIn && accountUsage.length ? <div className="space-y-3">{accountUsage.map((usage, index) => usage.percent !== null ? <div key={`${account.id}-usage-${index}`} className="space-y-2"><div className="flex items-center justify-between gap-3"><span className="text-2xs font-medium uppercase tracking-wide text-muted-foreground">{usage.label}</span><span data-testid={`provider-account-usage-${account.id}${index ? `-${index}` : ""}`} className="truncate text-xs text-muted-foreground" title={usage.text}>{usage.text}</span></div><div className="h-1.5 overflow-hidden rounded-full bg-muted" role="progressbar" aria-label={`${account.email} ${usage.label} usage remaining`} aria-valuemin={0} aria-valuemax={100} aria-valuenow={usage.percent}><div className={`h-full rounded-full transition-[width] ${usage.percent <= 20 ? "bg-warning" : "bg-primary"}`} style={{ width: `${usage.percent}%` }} /></div></div> : <div key={`${account.id}-usage-${index}`} data-testid={`provider-account-usage-${account.id}${index ? `-${index}` : ""}`} className="flex items-center gap-1.5 text-xs text-warning"><AlertCircle aria-hidden="true" className="size-3.5" />{usage.text}</div>)}</div> : <div className="flex items-center gap-1.5 text-xs text-muted-foreground">{account.signedIn ? <><Check aria-hidden="true" className="size-3.5 text-success" />Ready for new requests</> : <><AlertCircle aria-hidden="true" className="size-3.5 text-warning" />{t("settings.codexAccounts.signInAgain")}</>}</div>}
								</div>
							</Card>{primaryChangeCardFor(account)}{removalCardFor(account.id)}</div>;
						})}
						{query.data && !providerAccounts.some(account => account.signedIn) ? <div className="rounded-lg border border-dashed border-border/80 px-4 py-5 text-center"><p className="text-sm font-medium">{t("providerAccounts.emptyAccounts", { provider: providerName })}</p><p className="mx-auto mt-1 max-w-md text-xs leading-5 text-muted-foreground">{query.data?.defaults.find(defaultAccount => defaultAccount.provider === provider)?.managed ? t("providerAccounts.managedNeedsLogin") : t("providerAccounts.deviceSessionsContinue")}</p></div> : null}
					</div>
					</div>
				</div>;
			})}

			{message ? <p role="status" className="px-1 text-xs text-muted-foreground">{message}</p> : null}
		</div>
	</SettingsSection>;
}
