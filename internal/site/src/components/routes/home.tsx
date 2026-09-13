import { useLingui } from "@lingui/react/macro"
import { useStore } from "@nanostores/react"
import { getPagePath } from "@nanostores/router"
import { memo, Suspense, useEffect, useMemo } from "react"
import { $router, navigate } from "@/components/router"
import SystemsTable from "@/components/systems-table/systems-table"
import { ActiveAlerts } from "@/components/active-alerts"
import { FooterRepoLink } from "@/components/footer-repo-link"
import { $systems, $userSettings } from "@/lib/stores"
import { saveSettings } from "@/components/routes/settings/layout"

export default memo(() => {
	const { t } = useLingui()
	const systems = useStore($systems)
	const { singleNodeMode } = useStore($userSettings, { keys: ["singleNodeMode"] })

	useEffect(() => {
		document.title = `${t`All Systems`} / Beszel`
	}, [t])

	useEffect(() => {
		if (singleNodeMode && systems.length === 1) {
			navigate(getPagePath($router, "system", { id: systems[0].id }))
		} else if (singleNodeMode && systems.length > 1) {
			saveSettings({ singleNodeMode: false })
		}
	}, [systems, singleNodeMode])

	return useMemo(
		() => (
			<>
				<div className="flex flex-col gap-4">
					<ActiveAlerts />
					<Suspense>
						<SystemsTable />
					</Suspense>
				</div>
				<FooterRepoLink />
			</>
		),
		[]
	)
})
