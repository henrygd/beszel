import { t } from "@lingui/core/macro"
import { useEffect } from "react"
import SmartTable from "@/components/routes/system/smart-table"
import { ActiveAlerts } from "@/components/active-alerts"
import { FooterRepoLink } from "@/components/footer-repo-link"

export default function Smart() {
	useEffect(() => {
		document.title = `${t`S.M.A.R.T.`} / Beszel`
	}, [])

	return (
		<>
			<div className="grid gap-4">
				<div>
					<p className="eyebrow">{t`Fleet`}</p>
					<h1 className="mt-1.5 text-2xl font-semibold tracking-tight sm:text-[1.75rem]">S.M.A.R.T.</h1>
				</div>
				<ActiveAlerts />
				<SmartTable />
			</div>
			<FooterRepoLink />
		</>
	)
}
