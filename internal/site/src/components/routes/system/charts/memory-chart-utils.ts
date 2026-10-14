export function getMemoryChartTotal(systemStats?: readonly { stats?: { m?: number } }[]) {
	return systemStats?.at(-1)?.stats?.m ?? 0
}
