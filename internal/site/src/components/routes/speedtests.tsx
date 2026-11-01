import { useLingui } from "@lingui/react/macro"
import { memo, useEffect } from "react"
import SpeedtestsTable from "@/components/speedtests-table/speedtests-table"
import { FooterRepoLink } from "@/components/footer-repo-link"
import { useSpeedtests } from "@/lib/use-speedtests"
import { $allSystemsById } from "@/lib/stores"
import { supportsSpeedtests } from "@/lib/utils"
import { useStore } from "@nanostores/react"

export default memo(() => {
	const { t } = useLingui()
	const { speedtests, isLoading } = useSpeedtests({})
	const systems = useStore($allSystemsById)
	const visibleSpeedtests = speedtests.filter((speedtest) => {
		const system = systems[speedtest.system]
		return !system || supportsSpeedtests(system)
	})

	useEffect(() => {
		document.title = `${t`Speedtests`} / Beszel`
	}, [t])

	return (
		<>
			<SpeedtestsTable speedtests={visibleSpeedtests} isLoading={isLoading} />
			<FooterRepoLink />
		</>
	)
})
