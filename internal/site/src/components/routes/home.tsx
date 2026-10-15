import { useLingui } from "@lingui/react/macro"
import { memo, Suspense, useEffect, useMemo } from "react"
import SystemsTable from "@/components/systems-table/systems-table"
import { FooterRepoLink } from "@/components/footer-repo-link"

export default memo(() => {
	const { t } = useLingui()

	useEffect(() => {
		document.title = `${t`All Systems`} / Beszel`
	}, [t])

	return useMemo(
		() => (
			<>
				<Suspense>
					<SystemsTable />
				</Suspense>
				<FooterRepoLink />
			</>
		),
		[]
	)
})
