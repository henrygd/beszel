import { useStore } from "@nanostores/react"
import { useMemo } from "react"
import { sendInBatches } from "@/lib/api"
import { $tagsById } from "@/lib/stores"
import type { TagRecord } from "@/types"

// Badge classes per tag color (Tailwind colors). Class names must stay literal so Tailwind picks them up.
const tagColorClasses: Record<string, string> = {
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

/** Tag color names, in picker order */
export const tagColors = Object.keys(tagColorClasses)

// Generate a random color name
export function getRandomColor(): string {
	return tagColors[Math.floor(Math.random() * tagColors.length)]
}

// Get classes for a tag color (also used for the color picker swatches)
export function getTagColorClasses(color?: string): string {
	return tagColorClasses[color || "blue"] || tagColorClasses.blue
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

/** Tag records for the given tag ids, kept in sync with the tags store */
export function useTags(ids: string[] | undefined): TagRecord[] {
	const tagsById = useStore($tagsById)
	return useMemo(() => resolveTags(ids, tagsById), [ids, tagsById])
}

// Map of tag id -> records having that tag assigned, preserving record order
export function groupRecordsByTag<T extends Taggable>(records: T[]): Record<string, T[]> {
	const groups: Record<string, T[]> = {}
	for (const record of records) {
		for (const tagId of record.tags ?? []) {
			;(groups[tagId] ??= []).push(record)
		}
	}
	return groups
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
 * Add or remove a tag on records of the given collection so that exactly
 * desiredIds have it. Uses PocketBase's `tags+` / `tags-` modifiers so each
 * record's other tags are changed server side and never overwritten with stale data.
 */
export async function syncTagAssignments(
	collection: string,
	tagId: string,
	currentIds: string[],
	desiredIds: string[]
): Promise<void> {
	const updates: Record<string, string>[] = []
	for (const id of desiredIds) {
		if (!currentIds.includes(id)) updates.push({ id, "tags+": tagId })
	}
	for (const id of currentIds) {
		if (!desiredIds.includes(id)) updates.push({ id, "tags-": tagId })
	}
	await sendInBatches(updates, (batch, { id, ...data }) => batch.collection(collection).update(id, data))
}
