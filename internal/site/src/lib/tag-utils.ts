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
	red: "bg-red-100 text-red-700 dark:bg-red-950 dark:text-red-300",
	orange: "bg-orange-100 text-orange-700 dark:bg-orange-950 dark:text-orange-300",
	amber: "bg-amber-100 text-amber-700 dark:bg-amber-950 dark:text-amber-300",
	yellow: "bg-yellow-100 text-yellow-700 dark:bg-yellow-950 dark:text-yellow-300",
	lime: "bg-lime-100 text-lime-700 dark:bg-lime-950 dark:text-lime-300",
	green: "bg-green-100 text-green-700 dark:bg-green-950 dark:text-green-300",
	emerald: "bg-emerald-100 text-emerald-700 dark:bg-emerald-950 dark:text-emerald-300",
	teal: "bg-teal-100 text-teal-700 dark:bg-teal-950 dark:text-teal-300",
	cyan: "bg-cyan-100 text-cyan-700 dark:bg-cyan-950 dark:text-cyan-300",
	sky: "bg-sky-100 text-sky-700 dark:bg-sky-950 dark:text-sky-300",
	blue: "bg-blue-100 text-blue-700 dark:bg-blue-950 dark:text-blue-300",
	indigo: "bg-indigo-100 text-indigo-700 dark:bg-indigo-950 dark:text-indigo-300",
	violet: "bg-violet-100 text-violet-700 dark:bg-violet-950 dark:text-violet-300",
	purple: "bg-purple-100 text-purple-700 dark:bg-purple-950 dark:text-purple-300",
	fuchsia: "bg-fuchsia-100 text-fuchsia-700 dark:bg-fuchsia-950 dark:text-fuchsia-300",
	pink: "bg-pink-100 text-pink-700 dark:bg-pink-950 dark:text-pink-300",
	rose: "bg-rose-100 text-rose-700 dark:bg-rose-950 dark:text-rose-300",
}

// Swatch-only (background) classes for a tag color, used by the color picker
export const tagSwatchClasses: Record<string, string> = {
	red: "bg-red-100 dark:bg-red-950",
	orange: "bg-orange-100 dark:bg-orange-950",
	amber: "bg-amber-100 dark:bg-amber-950",
	yellow: "bg-yellow-100 dark:bg-yellow-950",
	lime: "bg-lime-100 dark:bg-lime-950",
	green: "bg-green-100 dark:bg-green-950",
	emerald: "bg-emerald-100 dark:bg-emerald-950",
	teal: "bg-teal-100 dark:bg-teal-950",
	cyan: "bg-cyan-100 dark:bg-cyan-950",
	sky: "bg-sky-100 dark:bg-sky-950",
	blue: "bg-blue-100 dark:bg-blue-950",
	indigo: "bg-indigo-100 dark:bg-indigo-950",
	violet: "bg-violet-100 dark:bg-violet-950",
	purple: "bg-purple-100 dark:bg-purple-950",
	fuchsia: "bg-fuchsia-100 dark:bg-fuchsia-950",
	pink: "bg-pink-100 dark:bg-pink-950",
	rose: "bg-rose-100 dark:bg-rose-950",
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
