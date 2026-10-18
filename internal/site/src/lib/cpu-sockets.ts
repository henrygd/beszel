/**
 * CPU model and tooltip prefix for the system info bar. Only multi-socket systems
 * get a prefix; 0 means the agent did not report a count.
 * `cpusLabel` is the translated "N CPUs" text, passed in so this stays testable.
 */
export function cpuSocketDisplay(cpuModel: string | undefined, sockets: number | undefined, cpusLabel: string) {
	if (!cpuModel || !sockets || sockets < 2) {
		return { model: cpuModel, labelPrefix: "" }
	}
	return { model: `${sockets}x ${cpuModel}`, labelPrefix: `${cpusLabel} / ` }
}
