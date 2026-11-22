import { useStore } from "@nanostores/react"
import { createContext, useContext, useMemo } from "react"
import { syncByNearestTime } from "@/lib/chart-sync"
import { $chartSync } from "@/lib/stores"

/**
 * recharts syncId for charts below this provider. Sheets provide their own group so hovering a
 * chart in a sheet doesn't open tooltips on the page charts behind it.
 */
export const ChartSyncGroup = createContext("beszel")

/** Hides the tooltip box on charts that aren't hovered, leaving only the cursor and active dots */
const crosshairClass = "[&:not(:hover)_.recharts-tooltip-wrapper]:hidden"

/** Returns recharts props and container class for the user's chart sync setting */
export function useChartSync() {
	const mode = useStore($chartSync)
	const syncId = useContext(ChartSyncGroup)
	return useMemo(() => {
		if (mode === "off") {
			return { syncProps: undefined, className: undefined }
		}
		return {
			syncProps: { syncId, syncMethod: syncByNearestTime },
			className: mode === "crosshair" ? crosshairClass : undefined,
		}
	}, [mode, syncId])
}
