import { useStore } from "@nanostores/react"
import { useEffect, useRef } from "react"
import { $alertNavigation, getAlertDestination, scrollToAlertSection } from "@/lib/alert-navigation"
import { $router } from "../../router"

export function useAlertNavigation({
	id,
	systemId,
	displayMode,
	tabs,
	chartsReady,
	setActiveTab,
}: {
	id: string
	systemId?: string
	displayMode: string
	tabs: string
	chartsReady: boolean
	setActiveTab: (tab: string) => void
}) {
	const page = useStore($router)
	const navigation = useStore($alertNavigation)
	const ref = useRef<HTMLDivElement>(null)
	const completed = useRef("")
	const alert = page?.route === "system" ? page.params.alert : undefined
	const destination = alert ? getAlertDestination(alert) : undefined

	useEffect(() => {
		if (!destination) {
			completed.current = ""
			return
		}
		const request = `${id}:${alert}:${navigation}:${displayMode}`
		if (completed.current === request) return
		// Wait for the requested system and any data-dependent tabs to become available.
		if (systemId !== id || !chartsReady) return
		if (displayMode === "tabs") {
			if (!tabs.split(",").includes(destination.tab)) {
				setActiveTab("core")
				return
			}
			setActiveTab(destination.tab)
		}
		const root = ref.current
		if (!root) return
		// Tables and some charts are mounted after their data or lazy imports arrive.
		return scrollToAlertSection(root, destination.section, () => {
			completed.current = request
		})
	}, [id, systemId, alert, navigation, displayMode, tabs, chartsReady, setActiveTab])

	return ref
}
