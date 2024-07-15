import { RecordModel } from 'pocketbase'

export interface SystemRecord extends RecordModel {
	name: string
	host: string
	status: 'up' | 'down' | 'paused' | 'pending'
	port: string
	info: SystemInfo
}

export interface SystemInfo {
	/** cpu percent */
	cpu: number
	/** cpu threads */
	t: number
	/** cpu cores */
	c: number
	/** cpu model */
	m: string
	/** operating system */
	o?: string
	/** uptime */
	u: number
	/** memory percent */
	mp: number
	/** disk percent */
	dp: number
}

export interface SystemStats {
	/** cpu percent */
	cpu: number
	/** total memory (gb) */
	m: number
	/** memory used (gb) */
	mu: number
	/** memory percent */
	mp: number
	/** memory buffer + cache (gb) */
	mb: number
	/** disk size (gb) */
	d: number
	/** disk used (gb) */
	du: number
	/** disk percent */
	dp: number
	/** disk read (mb) */
	dr: number
	/** disk write (mb) */
	dw: number
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
	info: SystemStats
}
