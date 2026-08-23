import { useLingui } from "@lingui/react/macro"
import { memo, useEffect, useMemo } from "react"
import ContainersTable from "@/components/containers-table/containers-table"
import { ActiveAlerts } from "@/components/active-alerts"
import { FooterRepoLink } from "@/components/footer-repo-link"

export default memo(() => {
	const { t } = useLingui()

	useEffect(() => {
		document.title = `${t`All Containers`} / Beszel`
	}, [t])

	return useMemo(
		() => (
			<>
				<div className="grid gap-4">
					<div>
						<p className="eyebrow">{t`Fleet`}</p>
						<h1 className="mt-1.5 text-2xl font-semibold tracking-tight sm:text-[1.75rem]">{t`All Containers`}</h1>
					</div>
					<ActiveAlerts />
					<ContainersTable />
				</div>
				<FooterRepoLink />
			</>
		),
		[t]
	)
})
