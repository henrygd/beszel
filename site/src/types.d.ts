import { RecordModel } from 'pocketbase'

export interface SystemRecord extends RecordModel {
	name: string
	ip: string
	active: boolean
	port: string
	stats: SystemStats
}

export interface SystemStats {
	/** cpu percent */
	c: number
	/** disk size (gb) */
	d: number
	/** disk percent */
	dp: number
	/** disk used (gb) */
	du: number
	/** total memory (gb) */
	m: number
	/** memory percent */
	mp: number
	/** memory buffer + cache (gb) */
	mb: number
	/** memory used (gb) */
	mu: number
}

export interface ContainerStatsRecord extends RecordModel {
	system: string
	stats: ContainerStats[]
}

interface ContainerStats {
	/** name */
	n: string
	/** cpu percent */
	c: number
	/** memory used (gb) */
	m: number
}

export interface SystemStatsRecord extends RecordModel {
	system: string
	stats: SystemStats
}
