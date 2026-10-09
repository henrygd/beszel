import { pb } from "@/lib/api"
import type { TagRecord } from "@/types"

// Tag color names (Tailwind colors)
export const tagColors = [
	"red",
	"orange",
	"amber",
	"yellow",
	"lime",
	"green",
	"emerald",
	"teal",
	"cyan",
	"sky",
	"blue",
	"indigo",
	"violet",
	"purple",
	"fuchsia",
	"pink",
	"rose",
]

// Tag color classes mapping
export const tagColorClasses: Record<string, string> = {
	red: "bg-red-500/15! text-red-600 dark:text-red-400",
	orange: "bg-orange-500/15! text-orange-600 dark:text-orange-400",
	amber: "bg-amber-500/15! text-amber-600 dark:text-amber-400",
	yellow: "bg-yellow-500/15! text-yellow-600 dark:text-yellow-400",
	lime: "bg-lime-500/15! text-lime-600 dark:text-lime-400",
	green: "bg-green-500/15! text-green-600 dark:text-green-400",
	emerald: "bg-emerald-500/15! text-emerald-600 dark:text-emerald-400",
	teal: "bg-teal-500/15! text-teal-600 dark:text-teal-400",
	cyan: "bg-cyan-500/15! text-cyan-600 dark:text-cyan-400",
	sky: "bg-sky-500/15! text-sky-600 dark:text-sky-400",
	blue: "bg-blue-500/15! text-blue-600 dark:text-blue-400",
	indigo: "bg-indigo-500/15! text-indigo-600 dark:text-indigo-400",
	violet: "bg-violet-500/15! text-violet-600 dark:text-violet-400",
	purple: "bg-purple-500/15! text-purple-600 dark:text-purple-400",
	fuchsia: "bg-fuchsia-500/15! text-fuchsia-600 dark:text-fuchsia-400",
	pink: "bg-pink-500/15! text-pink-600 dark:text-pink-400",
	rose: "bg-rose-500/15! text-rose-600 dark:text-rose-400",
}

// Swatch-only (background) classes for a tag color, used by the color picker
export const tagSwatchClasses: Record<string, string> = {
	red: "bg-red-500/15",
	orange: "bg-orange-500/15",
	amber: "bg-amber-500/15",
	yellow: "bg-yellow-500/15",
	lime: "bg-lime-500/15",
	green: "bg-green-500/15",
	emerald: "bg-emerald-500/15",
	teal: "bg-teal-500/15",
	cyan: "bg-cyan-500/15",
	sky: "bg-sky-500/15",
	blue: "bg-blue-500/15",
	indigo: "bg-indigo-500/15",
	violet: "bg-violet-500/15",
	purple: "bg-purple-500/15",
	fuchsia: "bg-fuchsia-500/15",
	pink: "bg-pink-500/15",
	rose: "bg-rose-500/15",
}

// Generate a random color name
export function getRandomColor(): string {
	return tagColors[Math.floor(Math.random() * tagColors.length)]
}

// Get classes for a tag color
export function getTagColorClasses(color?: string): string {
	return tagColorClasses[color || "blue"] || tagColorClasses.blue
}

// Get swatch-only (background) classes for a tag color
export function getSwatchColorClasses(color?: string): string {
	return tagSwatchClasses[color || "blue"] || tagSwatchClasses.blue
}

/** Any record with a `tags` relation field (systems, and later network monitors) */
export type Taggable = { id: string; tags?: string[] }

/** Resolve tag ids to tag records, dropping unknown ids, sorted by name */
export function resolveTags(ids: string[] | undefined, tagsById: Record<string, TagRecord | undefined>): TagRecord[] {
	if (!ids?.length) return []
	const tags: TagRecord[] = []
	for (const id of ids) {
		const tag = tagsById[id]
		if (tag) tags.push(tag)
	}
	return tags.sort((a, b) => a.name.localeCompare(b.name))
}

// Records that have the given tag assigned
export function getRecordsForTag<T extends Taggable>(records: T[], tagId: string): T[] {
	return records.filter((r) => r.tags?.includes(tagId))
}

// Whether a record has any of the given tag ids assigned
export function hasAnyTag(record: Taggable, tagIds: string[]): boolean {
	return !!record.tags?.length && tagIds.some((id) => record.tags?.includes(id))
}

// Filter records down to those having any of the given tag ids assigned.
// Returns the input array unchanged (same reference) when tagIds is empty.
export function filterByTags<T extends Taggable>(records: T[], tagIds: string[]): T[] {
	if (tagIds.length === 0) return records
	return records.filter((r) => hasAnyTag(r, tagIds))
}

// Build a map of tag id -> number of records having that tag assigned
export function buildTagCounts(records: Taggable[]): Record<string, number> {
	const counts: Record<string, number> = {}
	for (const record of records) {
		for (const tagId of record.tags ?? []) {
			counts[tagId] = (counts[tagId] ?? 0) + 1
		}
	}
	return counts
}

/**
 * Synchronize tag assignments with records of the given collection
 * Handles adding/removing the tag based on current vs desired state
 */
export async function syncTagAssignments(
	collection: string,
	tagId: string,
	currentIds: string[],
	desiredIds: string[],
	records: Taggable[]
): Promise<{ toAdd: string[]; toRemove: string[] }> {
	const toAdd = desiredIds.filter((id) => !currentIds.includes(id))
	const toRemove = currentIds.filter((id) => !desiredIds.includes(id))

	if (toAdd.length === 0 && toRemove.length === 0) return { toAdd, toRemove }

	const updates = [
		...toAdd.map((id) => {
			const record = records.find((r) => r.id === id)
			const newTags = [...(record?.tags || []), tagId]
			return pb.collection(collection).update(id, { tags: newTags })
		}),
		...toRemove.map((id) => {
			const record = records.find((r) => r.id === id)
			const newTags = (record?.tags || []).filter((t) => t !== tagId)
			return pb.collection(collection).update(id, { tags: newTags })
		}),
	]

	if (updates.length > 0) {
		await Promise.all(updates)
	}
	return { toAdd, toRemove }
}
