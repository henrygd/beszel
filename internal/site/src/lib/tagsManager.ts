import { pb } from "@/lib/api"
import { $tagsById } from "@/lib/stores"
import type { TagRecord } from "@/types"

const COLLECTION = pb.collection<TagRecord>("tags")

// biome-ignore lint/suspicious/noConfusingVoidType: typescript rocks
let unsub: (() => void) | undefined | void

/** Load all tags into the shared tags store */
export async function refresh() {
	try {
		const tags = await COLLECTION.getFullList()
		const tagsMap: Record<string, TagRecord> = {}
		for (const tag of tags) {
			tagsMap[tag.id] = tag
		}
		$tagsById.set(tagsMap)
	} catch (error: any) {
		// Ignore auto-cancellation errors
		if (error.isAbort || error.name === "AbortError") {
			return
		}
		console.error("Failed to load tags:", error)
	}
}

/** Subscribe to real-time tag updates */
export async function subscribe() {
	try {
		unsub = await COLLECTION.subscribe("*", ({ action, record }) => {
			if (action === "delete") {
				$tagsById.setKey(record.id, undefined as unknown as TagRecord)
			} else {
				$tagsById.setKey(record.id, record)
			}
		})
	} catch (error) {
		console.error("Failed to subscribe to tags collection:", error)
	}
}

/** Unsubscribe from real-time tag updates */
export const unsubscribe = () => (unsub = unsub?.())
