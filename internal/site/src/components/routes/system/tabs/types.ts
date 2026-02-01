import type { JSX, RefObject } from "react"
import type { ChartData, SystemRecord, SystemStatsRecord, UserSettings } from "@/types"
import type { useContainerChartConfigs } from "@/components/charts/hooks"

export interface BaseTabProps {
	chartData: ChartData
	grid: boolean
	dataEmpty: boolean
}

export interface CoreMetricsTabProps extends BaseTabProps {
	maxValSelect: JSX.Element | null
	showMax: boolean
	systemStats: SystemStatsRecord[]
	temperatureChartRef: RefObject<HTMLDivElement | null>
	maxValues: boolean
	userSettings: UserSettings
}

export interface DisksTabProps extends BaseTabProps {
	maxValSelect: JSX.Element | null
	showMax: boolean
	systemStats: SystemStatsRecord[]
	systemId: string
	userSettings: UserSettings
}

export interface GpuTabProps extends BaseTabProps {
	hasGpuPowerData: boolean
	hasGpuEnginesData: boolean
	systemStats: SystemStatsRecord[]
}

export interface ContainersTabProps extends BaseTabProps {
	containerFilterBar: JSX.Element | null
	containerChartConfigs: ReturnType<typeof useContainerChartConfigs>
	system: SystemRecord
}

export interface ServicesTabProps {
	systemId: string
}
