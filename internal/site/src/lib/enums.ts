/** Operating system */
export enum Os {
	Linux = 0,
	Darwin,
	Windows,
	FreeBSD,
}

/** Type of chart */
export enum ChartType {
	Memory,
	Disk,
	Network,
	CPU,
}

/** Unit of measurement */
export enum Unit {
	Bytes,
	Bits,
	Celsius,
	Fahrenheit,
	KB,
	MB,
	GB,
	TB,
}

/** Meter state for color */
export enum MeterState {
	Good,
	Warn,
	Crit,
}

/** System status states */
export enum SystemStatus {
	Up = "up",
	Down = "down",
	Pending = "pending",
	Paused = "paused",
}

/** Battery state */
export enum BatteryState {
	Unknown,
	Empty,
	Full,
	Charging,
	Discharging,
	Idle,
}

/** Time format */
export enum HourFormat {
	// Default = "Default",
	"12h" = "12h",
	"24h" = "24h",
}

/** Container health status */
export enum ContainerHealth {
	None,
	Starting,
	Healthy,
	Unhealthy,
}

export const ContainerHealthLabels = ["None", "Starting", "Healthy", "Unhealthy"] as const

/** Connection type */
export enum ConnectionType {
	SSH = 1,
	WebSocket,
}

export const connectionTypeLabels = ["", "SSH", "WebSocket"] as const

/** Systemd service state */
export enum ServiceStatus {
	Active,
	Inactive,
	Failed,
	Activating,
	Deactivating,
	Reloading,
}

export const ServiceStatusLabels = ["Active", "Inactive", "Failed", "Activating", "Deactivating", "Reloading"] as const

/** Systemd service sub state */
export enum ServiceSubState {
	Dead,
	Running,
	Exited,
	Failed,
	Unknown,
}

export const ServiceSubStateLabels = ["Dead", "Running", "Exited", "Failed", "Unknown"] as const
