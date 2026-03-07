import type { ChartData, SystemRecord } from "@/types"

export interface BaseTabProps {
	chartData: ChartData
	grid: boolean
}

export interface DisksTabProps extends BaseTabProps {
	systemId: string
}

export interface ContainersTabProps extends BaseTabProps {
	system: SystemRecord
}

export interface ServicesTabProps {
	systemId: string
}
