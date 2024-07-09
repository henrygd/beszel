import { RecordModel } from 'pocketbase'

export interface SystemRecord extends RecordModel {
	name: string
	ip: string
	active: boolean
	port: string
	stats: SystemStats
}

export interface SystemStats {
	cpu: number
	disk: number
	diskPct: number
	diskUsed: number
	mem: number
	memPct: number
	memUsed: number
}
