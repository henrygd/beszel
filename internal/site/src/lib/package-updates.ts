/**
 * Kind of version change between an installed and an available package version.
 * - major / minor / patch: first differing numeric component of the upstream version
 * - revision: same upstream version, only the distro packaging revision changed
 * - other: anything that can't be classified safely (unknown current version,
 *   epoch change, calendar versions, pre-release suffixes, downgrades)
 */
export type VersionChange = "major" | "minor" | "patch" | "revision" | "other"

interface SplitVersion {
	epoch: number
	upstream: string
	revision: string
}

/**
 * Splits a distro version string into epoch, upstream version and packaging revision.
 * Works for the Debian ("1:2.3.4-1ubuntu1"), RPM ("2:9.2.390-1.fc42"), pacman ("1.3.7-1")
 * and apk ("1.2.5-r3") formats. The revision follows the last "-".
 */
function splitVersion(version: string): SplitVersion {
	let epoch = 0
	const epochMatch = /^(\d+):/.exec(version)
	if (epochMatch) {
		epoch = Number(epochMatch[1])
		version = version.slice(epochMatch[0].length)
	}
	const dash = version.lastIndexOf("-")
	if (dash > 0) {
		return { epoch, upstream: version.slice(0, dash), revision: version.slice(dash + 1) }
	}
	return { epoch, upstream: version, revision: "" }
}

/**
 * Leading components at or above this look like dates or years (20240226, 2026b,
 * 2025.2.80), where a change in the first component is not a major upgrade.
 */
const CALENDAR_VERSION_MIN = 1000

/** Classifies the change from `current` to `available` as major, minor, patch or revision. */
export function classifyVersionChange(current?: string, available?: string): VersionChange {
	current = current?.trim()
	available = available?.trim()
	if (!current || !available || current === available) {
		return "other"
	}
	const from = splitVersion(current)
	const to = splitVersion(available)
	// a new epoch means the version scheme was reset, so the numbers aren't comparable
	if (from.epoch !== to.epoch) {
		return "other"
	}
	if (from.upstream === to.upstream) {
		return from.revision !== to.revision ? "revision" : "other"
	}
	const fromMatch = /^\d+(?:\.\d+)*/.exec(from.upstream)
	const toMatch = /^\d+(?:\.\d+)*/.exec(to.upstream)
	if (!fromMatch || !toMatch) {
		return "other"
	}
	const fromParts = fromMatch[0].split(".").map(Number)
	const toParts = toMatch[0].split(".").map(Number)
	if (fromParts[0] >= CALENDAR_VERSION_MIN || toParts[0] >= CALENDAR_VERSION_MIN) {
		return "other"
	}
	const length = Math.max(fromParts.length, toParts.length)
	for (let i = 0; i < length; i++) {
		const a = fromParts[i] ?? 0
		const b = toParts[i] ?? 0
		if (a === b) {
			continue
		}
		if (b < a) {
			return "other"
		}
		return i === 0 ? "major" : i === 1 ? "minor" : "patch"
	}
	// same numbers, so only a suffix such as "~rc1", "+dfsg" or a letter changed
	return "other"
}
