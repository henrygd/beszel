import type { RecordModel } from "pocketbase"
import type { Unit, Os, BatteryState, HourFormat, ConnectionType, ServiceStatus, ServiceSubState } from "@/lib/enums"

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
	updated: string
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
	/** load average 1 minute */
	l1?: number
	/** load average 5 minutes */
	l5?: number
	/** load average 15 minutes */
	l15?: number
	/** load average */
	la?: [number, number, number]
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
	/** bandwidth bytes */
	bb?: number
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
	/** cpu info (array of cpu objects) */
	c?: CpuInfo[]
	/** os info (array of os objects) */
	o?: OsInfo[]
	/** operating system */
	os?: Os
	/** connection type */
	ct?: ConnectionType
}

export interface SystemStats {
	/** cpu percent */
	cpu: number
	/** peak cpu */
	cpum?: number
	/** cpu breakdown [user, system, iowait, steal, idle] (0-100 integers) */
	cpub?: number[]
	/** per-core cpu usage [CPU0..] (0-100 integers) */
	cpus?: number[]
	// TODO: remove these in future release in favor of la
	/** load average 1 minute */
	l1?: number
	/** load average 5 minutes */
	l5?: number
	/** load average 15 minutes */
	l15?: number
	/** load average */
	la?: [number, number, number]
	/** total memory (gb) */
	m: number
	/** memory used (gb) */
	mu: number
	/** memory percent */
	mp: number
	/** memory buffer + cache (gb) */
	mb: number
	/** max used memory (gb) */
	mm?: number
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
	/** disk I/O bytes [read, write] */
	dio?: [number, number]
	/** max disk I/O bytes [read, write] */
	diom?: [number, number]
	/** network sent (mb) */
	ns: number
	/** network received (mb) */
	nr: number
	/** bandwidth bytes [sent, recv] */
	b?: [number, number]
	/** max network sent (mb) */
	nsm?: number
	/** max network received (mb) */
	nrm?: number
	/** max network sent (bytes) */
	bm?: [number, number]
	/** temperatures */
	t?: Record<string, number>
	/** extra filesystems */
	efs?: Record<string, ExtraFsStats>
	/** GPU data */
	g?: Record<string, GPUData>
	/** battery percent and state */
	bat?: [number, BatteryState]
	/** network interfaces [upload bytes, download bytes, total upload bytes, total download bytes] */
	ni?: Record<string, [number, number, number, number]>
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
	/** power package (w) */
	pp?: number
	/** engines */
	e?: Record<string, number>
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
	/** read per second (bytes) */
	rb: number
	/** write per second (bytes) */
	wb: number
	/** max read per second (bytes) */
	rbm: number
	/** max write per second (mb) */
	wbm: number
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
	value: number
	min: number
	// user: string
}

export interface AlertsHistoryRecord extends RecordModel {
	alert: string
	user: string
	system: string
	name: string
	val: number
	created: string
	resolved?: string | null
}

export interface ContainerRecord extends RecordModel {
	id: string
	system: string
	name: string
	image: string
	cpu: number
	memory: number
	net: number
	health: number
	status: string
	updated: number
}

export type ChartTimes = "1m" | "1h" | "12h" | "24h" | "1w" | "30d"

export interface ChartTimeData {
	[key: string]: {
		type: "1m" | "10m" | "20m" | "120m" | "480m"
		expectedInterval: number
		label: () => string
		ticks?: number
		format: (timestamp: string) => string
		getOffset: (endTime: Date) => Date
		minVersion?: string
	}
}

export interface UserSettings {
	chartTime: ChartTimes
	emails?: string[]
	webhooks?: string[]
	unitTemp?: Unit
	unitNet?: Unit
	unitDisk?: Unit
	colorWarn?: number
	colorCrit?: number
	hourFormat?: HourFormat
}

type ChartDataContainer = {
	created: number | null
} & {
	[key: string]: key extends "created" ? never : ContainerStats
}

export interface SemVer {
	major: number
	minor: number
	patch: number
}

export interface ChartData {
	agentVersion: SemVer
	systemStats: SystemStatsRecord[]
	containerData: ChartDataContainer[]
	orientation: "right" | "left"
	ticks: number[]
	domain: number[]
	chartTime: ChartTimes
}

export interface AlertInfo {
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

export type AlertMap = Record<string, Map<string, AlertRecord>>

export interface SmartData {
	/** model family */
	// mf?: string
	/** model name */
	mn?: string
	/** serial number */
	sn?: string
	/** firmware version */
	fv?: string
	/** capacity */
	c?: number
	/** smart status */
	s?: string
	/** disk name (like /dev/sda) */
	dn?: string
	/** disk type */
	dt?: string
	/** temperature */
	t?: number
	/** attributes */
	a?: SmartAttribute[]
}

export interface SmartAttribute {
	/** id */
	id?: number
	/** name */
	n: string
	/** value */
	v: number
	/** worst */
	w?: number
	/** threshold */
	t?: number
	/** raw value */
	rv?: number
	/** raw string */
	rs?: string
	/** when failed */
	wf?: string
}

export interface SystemdRecord extends RecordModel {
	system: string
	name: string
	state: ServiceStatus
	sub: ServiceSubState
	cpu: number
	cpuPeak: number
	memory: number
	memPeak: number
	updated: number
}

export interface SystemdServiceDetails {
	AccessSELinuxContext: string;
	ActivationDetails: any[];
	ActiveEnterTimestamp: number;
	ActiveEnterTimestampMonotonic: number;
	ActiveExitTimestamp: number;
	ActiveExitTimestampMonotonic: number;
	ActiveState: string;
	After: string[];
	AllowIsolate: boolean;
	AssertResult: boolean;
	AssertTimestamp: number;
	AssertTimestampMonotonic: number;
	Asserts: any[];
	Before: string[];
	BindsTo: any[];
	BoundBy: any[];
	CPUUsageNSec: number;
	CanClean: any[];
	CanFreeze: boolean;
	CanIsolate: boolean;
	CanLiveMount: boolean;
	CanReload: boolean;
	CanStart: boolean;
	CanStop: boolean;
	CollectMode: string;
	ConditionResult: boolean;
	ConditionTimestamp: number;
	ConditionTimestampMonotonic: number;
	Conditions: any[];
	ConflictedBy: any[];
	Conflicts: string[];
	ConsistsOf: any[];
	DebugInvocation: boolean;
	DefaultDependencies: boolean;
	Description: string;
	Documentation: string[];
	DropInPaths: any[];
	ExecMainPID: number;
	FailureAction: string;
	FailureActionExitStatus: number;
	Following: string;
	FragmentPath: string;
	FreezerState: string;
	Id: string;
	IgnoreOnIsolate: boolean;
	InactiveEnterTimestamp: number;
	InactiveEnterTimestampMonotonic: number;
	InactiveExitTimestamp: number;
	InactiveExitTimestampMonotonic: number;
	InvocationID: string;
	Job: Array<number | string>;
	JobRunningTimeoutUSec: number;
	JobTimeoutAction: string;
	JobTimeoutRebootArgument: string;
	JobTimeoutUSec: number;
	JoinsNamespaceOf: any[];
	LoadError: string[];
	LoadState: string;
	MainPID: number;
	Markers: any[];
	MemoryCurrent: number;
	MemoryLimit: number;
	MemoryPeak: number;
	NRestarts: number;
	Names: string[];
	NeedDaemonReload: boolean;
	OnFailure: any[];
	OnFailureJobMode: string;
	OnFailureOf: any[];
	OnSuccess: any[];
	OnSuccessJobMode: string;
	OnSuccessOf: any[];
	PartOf: any[];
	Perpetual: boolean;
	PropagatesReloadTo: any[];
	PropagatesStopTo: any[];
	RebootArgument: string;
	Refs: any[];
	RefuseManualStart: boolean;
	RefuseManualStop: boolean;
	ReloadPropagatedFrom: any[];
	RequiredBy: any[];
	Requires: string[];
	RequiresMountsFor: any[];
	Requisite: any[];
	RequisiteOf: any[];
	Result: string;
	SliceOf: any[];
	SourcePath: string;
	StartLimitAction: string;
	StartLimitBurst: number;
	StartLimitIntervalUSec: number;
	StateChangeTimestamp: number;
	StateChangeTimestampMonotonic: number;
	StopPropagatedFrom: any[];
	StopWhenUnneeded: boolean;
	SubState: string;
	SuccessAction: string;
	SuccessActionExitStatus: number;
	SurviveFinalKillSignal: boolean;
	TasksCurrent: number;
	TasksMax: number;
	Transient: boolean;
	TriggeredBy: string[];
	Triggers: any[];
	UnitFilePreset: string;
	UnitFileState: string;
	UpheldBy: any[];
	Upholds: any[];
	WantedBy: any[];
	Wants: string[];
	WantsMountsFor: any[];
}