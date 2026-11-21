/** Keep saved selections visible even when a sensor is absent from the latest sample. */
export function temperatureSensorNames(
	thresholds: Record<string, number>,
	temperatures?: Record<string, number>
): string[] {
	return [...new Set([...Object.keys(thresholds), ...Object.keys(temperatures ?? {})])].sort((a, b) =>
		a.localeCompare(b)
	)
}

/** Numeric inputs must never send empty, nonfinite, or out-of-range thresholds. */
export function temperatureThresholdInput(input: string, min = 1, max = 99): number | undefined {
	if (!input.trim()) return
	const value = Number(input)
	if (!Number.isFinite(value)) return
	return Math.max(min, Math.min(value, max))
}
