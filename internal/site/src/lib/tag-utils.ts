import { pb } from "@/lib/api"
import type { SystemRecord, TagRecord } from "@/types"

/**
 * Synchronize tag assignments with systems
 * Handles adding/removing tags from systems based on current vs desired state
 */
export async function syncTagAssignments(
	tagId: string,
	currentSystemIds: string[],
	desiredSystemIds: string[],
	systems: SystemRecord[]
): Promise<void> {
	const toAdd = desiredSystemIds.filter((id) => !currentSystemIds.includes(id))
	const toRemove = currentSystemIds.filter((id) => !desiredSystemIds.includes(id))

	if (toAdd.length === 0 && toRemove.length === 0) return

	const updates = [
		...toAdd.map((systemId) => {
			const system = systems.find((s) => s.id === systemId)
			const newTags = [...(system?.tags || []), tagId]
			return pb.collection("systems").update(systemId, { tags: newTags })
		}),
		...toRemove.map((systemId) => {
			const system = systems.find((s) => s.id === systemId)
			const newTags = (system?.tags || []).filter((t) => t !== tagId)
			return pb.collection("systems").update(systemId, { tags: newTags })
		}),
	]

	if (updates.length > 0) {
		await Promise.all(updates)
	}
}

/**
 * Update local system state after tag assignment changes
 */
export function updateSystemsStateAfterTagAssignment(
	systems: SystemRecord[],
	tagId: string,
	toAdd: string[],
	toRemove: string[]
): SystemRecord[] {
	return systems.map((s) => {
		if (toAdd.includes(s.id)) {
			return { ...s, tags: [...(s.tags || []), tagId] }
		}
		if (toRemove.includes(s.id)) {
			return { ...s, tags: (s.tags || []).filter((t) => t !== tagId) }
		}
		return s
	})
}
