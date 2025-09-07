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
