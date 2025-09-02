import { SystemRecord } from "@/types"
import { PreinitializedMapStore } from "nanostores"
import { pb, verifyAuth } from "@/lib/api"
import {
	$allSystemsByName,
	$upSystems,
	$downSystems,
	$pausedSystems,
	$allSystemsById,
	$longestSystemNameLen,
} from "@/lib/stores"
import { updateFavicon, FAVICON_DEFAULT, FAVICON_GREEN, FAVICON_RED } from "@/lib/utils"
import { SystemStatus } from "./enums"

const COLLECTION = pb.collection<SystemRecord>("systems")
const FIELDS_DEFAULT = "id,name,host,port,info,status"

/** Maximum system name length for display purposes */
const MAX_SYSTEM_NAME_LENGTH = 20

let initialized = false
let unsub: (() => void) | undefined | void

/** Initialize the systems manager and set up listeners */
export function init() {
	if (initialized) {
		return
	}
	initialized = true

	// sync system stores on change
	$allSystemsByName.listen((newSystems, oldSystems, changedKey) => {
		const oldSystem = oldSystems[changedKey]
		const newSystem = newSystems[changedKey]

		// if system is undefined (deleted), remove it from the stores
		if (oldSystem && !newSystem?.id) {
			removeFromStore(oldSystem, $upSystems)
			removeFromStore(oldSystem, $downSystems)
			removeFromStore(oldSystem, $pausedSystems)
			removeFromStore(oldSystem, $allSystemsById)
		}

		if (!newSystem) {
			onSystemsChanged(newSystems, undefined)
			return
		}

		const newStatus = newSystem.status
		if (newStatus === SystemStatus.Up) {
			$upSystems.setKey(newSystem.id, newSystem)
			removeFromStore(newSystem, $downSystems)
			removeFromStore(newSystem, $pausedSystems)
		} else if (newStatus === SystemStatus.Down) {
			$downSystems.setKey(newSystem.id, newSystem)
			removeFromStore(newSystem, $upSystems)
			removeFromStore(newSystem, $pausedSystems)
		} else if (newStatus === SystemStatus.Paused) {
			$pausedSystems.setKey(newSystem.id, newSystem)
			removeFromStore(newSystem, $upSystems)
			removeFromStore(newSystem, $downSystems)
		}

		// run things that need to be done when systems change
		onSystemsChanged(newSystems, newSystem)
	})
}

/** Update the longest system name length and favicon based on system status */
function onSystemsChanged(_: Record<string, SystemRecord>, changedSystem: SystemRecord | undefined) {
	const upSystemsStore = $upSystems.get()
	const downSystemsStore = $downSystems.get()
	const upSystems = Object.values(upSystemsStore)
	const downSystems = Object.values(downSystemsStore)

	// Update longest system name length
	const longestName = $longestSystemNameLen.get()
	const nameLen = Math.min(MAX_SYSTEM_NAME_LENGTH, changedSystem?.name.length || 0)
	if (nameLen > longestName) {
		$longestSystemNameLen.set(nameLen)
	}

	// Update favicon based on system status
	if (downSystems.length > 0) {
		updateFavicon(FAVICON_RED)
	} else if (upSystems.length > 0) {
		updateFavicon(FAVICON_GREEN)
	} else {
		updateFavicon(FAVICON_DEFAULT)
	}
}

/** Fetch systems from collection */
async function fetchSystems(): Promise<SystemRecord[]> {
	try {
		return await COLLECTION.getFullList({ sort: "+name", fields: FIELDS_DEFAULT })
	} catch (error) {
		console.error("Failed to fetch systems:", error)
		return []
	}
}

// Store management functions
/** Add system to both name and ID stores */
export function add(system: SystemRecord) {
	$allSystemsByName.setKey(system.name, system)
	$allSystemsById.setKey(system.id, system)
}

/** Remove system from stores */
export function remove(system: SystemRecord) {
	removeFromStore(system, $allSystemsByName)
	removeFromStore(system, $allSystemsById)
	removeFromStore(system, $upSystems)
	removeFromStore(system, $downSystems)
	removeFromStore(system, $pausedSystems)
}

/** Remove system from specific store */
function removeFromStore(system: SystemRecord, store: PreinitializedMapStore<Record<string, SystemRecord>>) {
	const key = store === $allSystemsByName ? system.name : system.id
	store.setKey(key, undefined as any)
}

/** Action functions for subscription */
const actionFns: Record<string, (system: SystemRecord) => void> = {
	create: add,
	update: add,
	delete: remove,
}

/** Subscribe to real-time system updates from the collection */
export async function subscribe() {
	try {
		unsub = await COLLECTION.subscribe("*", ({ action, record }) => actionFns[action]?.(record), {
			fields: FIELDS_DEFAULT,
		})
	} catch (error) {
		console.error("Failed to subscribe to systems collection:", error)
	}
}

/** Refresh all systems with latest data from the hub */
export async function refresh(records: SystemRecord[] = []) {
	if (records.length) {
		for (const record of records) {
			add(record)
		}
		return
	}
	try {
		const records = await fetchSystems()
		if (!records.length) {
			// No systems found, verify authentication
			verifyAuth()
			return
		}
		for (const record of records) {
			add(record)
		}
	} catch (error) {
		console.error("Failed to refresh systems:", error)
	}
}

/** Unsubscribe from real-time system updates */
export const unsubscribe = () => (unsub = unsub?.())
