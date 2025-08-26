import { RecordModel } from "pocketbase"
import { Os } from "./lib/enums"

// global window properties
declare global {
	var BESZEL: {
		BASE_PATH: string
		HUB_VERSION: string
		HUB_URL: string
	}
}

export interface FingerprintRecord extends RecordModel {
	id: string
	system: string
	fingerprint: string
	token: string
	expand: {
		system: {
			name: string
		}
	}
}

export interface SystemRecord extends RecordModel {
	name: string
	host: string
	status: "up" | "down" | "paused" | "pending"
	port: string
	info: systemInfo
	v: string
}

export interface CpuInfo {
	m: string
	s: string
	a: string
	c: number
	t: number
}

export interface OsInfo {
	f: string
	v: string
	k: string
}

export interface NetworkLocationInfo {
	ip?: string
	isp?: string
	asn?: string
}

export interface systemInfo {
	/** hostname */
	h: string
	/** kernel **/
	k?: string
	/** cpu percent */
	cpu: number
	/** cpu threads */
	t?: number
	/** cpu cores */
	c: number
	/** cpu model */
	m: string
	/** load average 5 minutes */
	l5?: number
	/** load average 15 minutes */
	l15?: number
	/** operating system */
	o?: string
	/** uptime */
	u: number
	/** memory percent */
	mp: number
	/** disk percent */
	dp: number
	/** bandwidth (mb) */
	b: number
	/** agent version */
	v: string
	/** system is using podman */
	p?: boolean
	/** highest gpu utilization */
	g?: number
	/** dashboard display temperature */
	dt?: number
	/** disks info (array of block devices with model/vendor/serial) */
	d?: { n: string; m?: string; v?: string; serial?: string }[]
	/** networks info (array of network interfaces with vendor/model/capabilities) */
	n?: { n: string; v?: string; m?: string; s?: string }[]
	/** memory info (array with total property) */
	m?: { t: string }[]
	/** OS name (from /etc/os-release NAME) */
	onr?: string
	/** OS version id (from /etc/os-release VERSION_ID) */
	ovid?: string
	/** cpu info (array of cpu objects) */
	c?: CpuInfo[]
	/** os info (array of os objects) */
	o?: OsInfo[]
	/** network location info (array of network location objects) */
	nl?: NetworkLocationInfo[]
}

export interface SystemStats {
	/** cpu percent */
	cpu: number
	/** peak cpu */
	cpum?: number
	/** load average 1 minute */
	l1?: number
	/** load average 5 minutes */
	l5?: number
	/** load average 15 minutes */
	l15?: number
	/** total memory (gb) */
	m: number
	/** memory used (gb) */
	mu: number
	/** memory percent */
	mp: number
	/** memory buffer + cache (gb) */
	mb: number
	/** zfs arc memory (gb) */
	mz?: number
	/** swap space (gb) */
	s: number
	/** swap used (gb) */
	su: number
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
	/** max disk read (mb) */
	drm?: number
	/** max disk write (mb) */
	dwm?: number
	/** network sent (mb) */
	ns: number
	/** network received (mb) */
	nr: number
	/** max network sent (mb) */
	nsm?: number
	/** max network received (mb) */
	nrm?: number
	/** temperatures */
	t?: Record<string, number>
	/** extra filesystems */
	efs?: Record<string, ExtraFsStats>
	/** GPU data */
	g?: Record<string, GPUData>
}

export interface GPUData {
	/** name */
	n: string
	/** memory used (mb) */
	mu?: number
	/** memory total (mb) */
	mt?: number
	/** usage (%) */
	u: number
	/** power (w) */
	p?: number
}

export interface ExtraFsStats {
	/** disk size (gb) */
	d: number
	/** disk used (gb) */
	du: number
	/** total read (mb) */
	r: number
	/** total write (mb) */
	w: number
	/** max read (mb) */
	rm: number
	/** max write (mb) */
	wm: number
}

export interface ContainerStatsRecord extends RecordModel {
	system: string
	stats: ContainerStats[]
	created: string | number
}

interface ContainerStats {
	/** name */
	n: string
	/** cpu percent */
	c: number
	/** memory used (gb) */
	m: number
	// network sent (mb)
	ns: number
	// network received (mb)
	nr: number
}

export interface SystemStatsRecord extends RecordModel {
	system: string
	stats: SystemStats
	created: string | number
}

export interface AlertRecord extends RecordModel {
	id: string
	system: string
	name: string
	triggered: boolean
	sysname?: string
	// user: string
}

export type ChartTimes = "1h" | "12h" | "24h" | "1w" | "30d"

export interface ChartTimeData {
	[key: string]: {
		type: "1m" | "10m" | "20m" | "120m" | "480m"
		expectedInterval: number
		label: () => string
		ticks?: number
		format: (timestamp: string) => string
		getOffset: (endTime: Date) => Date
	}
}

export type UserSettings = {
	// lang?: string
	chartTime: ChartTimes
	emails?: string[]
	webhooks?: string[]
}

type ChartDataContainer = {
	created: number | null
} & {
	[key: string]: key extends "created" ? never : ContainerStats
}

export interface ChartData {
	systemStats: SystemStatsRecord[]
	containerData: ChartDataContainer[]
	orientation: "right" | "left"
	ticks: number[]
	domain: number[]
	chartTime: ChartTimes
}

interface AlertInfo {
	name: () => string
	unit: string
	icon: any
	desc: () => string
	max?: number
	min?: number
	step?: number
	start?: number
	/** Single value description (when there's only one value, like status) */
	singleDesc?: () => string
}
