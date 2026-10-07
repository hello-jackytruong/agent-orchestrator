import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, UserRound } from "lucide-react";
import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import type { components } from "../../api/schema";
import { apiClient, apiErrorMessage } from "../lib/api-client";
import { providerAccountsKey, useProviderAccounts } from "../hooks/useProviderAccounts";
import {
	DropdownMenuItem,
	DropdownMenuSub,
	DropdownMenuSubContent,
	DropdownMenuSubTrigger,
} from "./ui/dropdown-menu";

type SessionRoute = components["schemas"]["SessionProviderAccountResponse"];

export function SessionProviderAccountMenuItem({ sessionId }: { sessionId: string }) {
	const { t } = useTranslation();
	const accounts = useProviderAccounts();
	const cache = useQueryClient();
	const [pending, setPending] = useState(false);
	const [error, setError] = useState("");
	const selecting = useRef(false);
	const route = useQuery({
		queryKey: ["session-provider-account", sessionId],
		refetchInterval: 5000,
		queryFn: async () => {
			const { data, error: responseError } = await apiClient.GET(
				"/api/v1/sessions/{sessionId}/provider-account",
				{ params: { path: { sessionId } } },
			);
			if (responseError) throw new Error(apiErrorMessage(responseError));
			return data!;
		},
	});
	if (!route.data?.managed) return null;

	const choices = (accounts.data?.accounts ?? []).filter(
		(account) => account.provider === route.data?.provider && account.signedIn,
	);
	const current = choices.find((account) => account.id === route.data?.accountId);
	async function switchAccount(accountId: string) {
		if (accountId === route.data?.accountId) return;
		setPending(true);
		setError("");
		try {
			const { data, error: responseError } = await apiClient.PUT(
				"/api/v1/sessions/{sessionId}/provider-account",
				{ params: { path: { sessionId } }, body: { accountId } },
			);
			if (responseError) throw new Error(apiErrorMessage(responseError));
			cache.setQueryData<SessionRoute>(["session-provider-account", sessionId], data);
			void cache.invalidateQueries({ queryKey: providerAccountsKey });
		} catch (switchError) {
			setError(switchError instanceof Error ? switchError.message : t("providerAccounts.sessionChangeFailed"));
		} finally {
			setPending(false);
			selecting.current = false;
		}
	}
	function selectAccount(accountId: string) {
		if (selecting.current) return;
		selecting.current = true;
		void switchAccount(accountId);
	}

	return (
		<DropdownMenuSub>
			<DropdownMenuSubTrigger disabled={pending}>
				<UserRound aria-hidden="true" className="size-3.5" />
				<span>{t("providerAccounts.switchAccount")}</span>
				{current ? <span className="ml-auto max-w-28 truncate text-2xs text-passive">{current.displayName || current.email}</span> : null}
			</DropdownMenuSubTrigger>
			<DropdownMenuSubContent className="min-w-56 text-xs">
				{choices.length ? choices.map((account) => (
					<DropdownMenuItem
						key={account.id}
						disabled={pending || account.id === route.data?.accountId}
						onClick={() => selectAccount(account.id)}
						onSelect={() => selectAccount(account.id)}
					>
						<Check aria-hidden="true" className={account.id === route.data?.accountId ? "size-3.5" : "invisible size-3.5"} />
						<span className="min-w-0 flex-1 truncate">{account.displayName || account.email}</span>
						{account.primary ? <span className="text-2xs text-passive">{t("providerAccounts.default")}</span> : null}
					</DropdownMenuItem>
				)) : (
					<DropdownMenuItem disabled>{t("providerAccounts.noAccountsAvailable")}</DropdownMenuItem>
				)}
				{route.data.loginRequired ? <DropdownMenuItem disabled>{t("providerAccounts.loginRequired")}</DropdownMenuItem> : null}
				{error ? <DropdownMenuItem disabled className="text-destructive">{error}</DropdownMenuItem> : null}
			</DropdownMenuSubContent>
		</DropdownMenuSub>
	);
}
