import { useEffect } from "react"
import NutTable from "@/components/routes/system/nut-table"
import { ActiveAlerts } from "@/components/active-alerts"
import { FooterRepoLink } from "@/components/footer-repo-link"

export default function Nut() {
	useEffect(() => {
		document.title = `UPS / PDU / Beszel`
	}, [])

	return (
		<>
			<div className="grid gap-4">
				<ActiveAlerts />
				<NutTable />
			</div>
			<FooterRepoLink />
		</>
	)
}