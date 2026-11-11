import type { SystemStats } from "@/types"

/** Select independent peaks without changing cached averages or filling chart gaps. */
export function selectChartMax(stats: SystemStats): SystemStats {
	const peaks = stats.mx
	const legacy = {
		...stats,
		cpum: stats.cpum ?? stats.cpu,
		memm: stats.memm ?? stats.mem?.[1],
		mm: stats.mm ?? stats.mu,
		bm: stats.bm ?? stats.b,
		diom: stats.diom ?? stats.dio,
		diosm: stats.diosm ?? stats.dios,
		nsm: stats.nsm ?? stats.ns,
		nrm: stats.nrm ?? stats.nr,
	}
	if (!peaks) return legacy
	return {
		...legacy,
		...peaks,
		ni: peaks.ni
			? Object.fromEntries(
					Object.entries(peaks.ni).map(([key, value]) => [
						key,
						[value[0], value[1], stats.ni?.[key]?.[2] ?? 0, stats.ni?.[key]?.[3] ?? 0],
					])
				)
			: stats.ni,
		efs: mergeEntries(stats.efs, peaks.efs, (v) => ({ du: v.du })),
		g: mergeEntries(stats.g, peaks.g, (v) => ({ u: v.u, mu: v.mu, p: v.p, pp: v.pp, e: v.e })),
		z: mergeEntries(stats.z, peaks.z, (v) => ({ du: v.du, rb: v.rb, wb: v.wb })),
	}
}

function mergeEntries<T>(
	average: Record<string, T> | undefined,
	peaks: Record<string, T> | undefined,
	project: (value: T) => Partial<T>
) {
	if (!peaks) return average
	const result = { ...average }
	for (const [key, value] of Object.entries(peaks)) result[key] = { ...average?.[key], ...project(value) } as T
	return result
}
