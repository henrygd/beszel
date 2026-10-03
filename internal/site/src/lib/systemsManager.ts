/** biome-ignore-all lint/suspicious/noAssignInExpressions: it's fine :) */
import type { PreinitializedMapStore } from "nanostores"
import { pb } from "@/lib/api"
import {
	$allSystemsById,
	$allSystemsByName,
	$downSystems,
	$longestSystemName,
	$pausedSystems,
	$textMeasureVersion,
	$upSystems,
} from "@/lib/stores"
import { isVisuallyLonger, updateFavicon } from "@/lib/utils"
import type { SystemRecord } from "@/types"
import { SystemStatus } from "./enums"

const COLLECTION = pb.collection<SystemRecord>("systems")
const FIELDS_DEFAULT = "id,name,host,port,info,status,tags"

let initialized = false
// biome-ignore lint/suspicious/noConfusingVoidType: typescript rocks
let unsub: (() => void) | undefined | void

/** Initialize the systems manager and set up listeners */
export function init() {
	if (initialized) {
		return
	}
	initialized = true

	// sync system stores on change
	$allSystemsById.listen((newSystems, oldSystems, changedKey) => {
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
			onSystemsChanged(newSystems, newSystem, oldSystem)
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
		} else if (newStatus === SystemStatus.Pending) {
			removeFromStore(newSystem, $upSystems)
			removeFromStore(newSystem, $downSystems)
			removeFromStore(newSystem, $pausedSystems)
		}

		// run things that need to be done when systems change
		onSystemsChanged(newSystems, newSystem, oldSystem)
	})

	// widths measured with the fallback font may rank names differently, so recompute once they're invalidated
	$textMeasureVersion.listen(() => {
		$longestSystemName.set(findLongestName($allSystemsById.get()))
	})
}

/** Update the longest system name string and favicon based on system status */
function onSystemsChanged(systems: Record<string, SystemRecord>, newSystem?: SystemRecord, oldSystem?: SystemRecord) {
	const downSystemsStore = $downSystems.get()
	const downSystems = Object.values(downSystemsStore)

	// if the old system's old name was the longest, we need to find the new longest name
	// otherwise, if the changed system's new name is longer than the current longest, update it
	const longestName = $longestSystemName.get()
	if (oldSystem?.name === longestName && oldSystem.name !== newSystem?.name) {
		$longestSystemName.set(findLongestName(systems))
	} else if (newSystem && newSystem.name !== longestName && isVisuallyLonger(newSystem.name, longestName)) {
		$longestSystemName.set(newSystem.name)
	}

	updateFavicon(downSystems.length)
}

/** Find the visually longest system name */
function findLongestName(systems: Record<string, SystemRecord>): string {
	let longest = ""
	for (const id in systems) {
		if (isVisuallyLonger(systems[id].name, longest)) {
			longest = systems[id].name
		}
	}
	return longest
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

/** Makes sure the system has valid info object and throws if not */
function validateSystemInfo(system: SystemRecord) {
	if (!("cpu" in system.info)) {
		throw new Error(`${system.name} has no CPU info`)
	}
}

/** Add system to both name and ID stores */
export function add(system: SystemRecord) {
	try {
		validateSystemInfo(system)
		$allSystemsByName.setKey(system.name, system)
		$allSystemsById.setKey(system.id, system)
	} catch (error) {
		console.error(error)
	}
}

/** Update system in stores */
export function update(system: SystemRecord) {
	try {
		validateSystemInfo(system)
		// if name changed, make sure old name is removed from the name store
		const oldName = $allSystemsById.get()[system.id]?.name
		if (oldName !== system.name) {
			$allSystemsByName.setKey(oldName, undefined as unknown as SystemRecord)
		}
		add(system)
	} catch (error) {
		console.error(error)
	}
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
	store.setKey(key, undefined as unknown as SystemRecord)
}

/** Action functions for subscription */
const actionFns: Record<string, (system: SystemRecord) => void> = {
	create: add,
	update: update,
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
export async function refresh() {
	try {
		const records = await fetchSystems()
		for (const record of records) {
			add(record)
		}
	} catch (error) {
		console.error("Failed to refresh systems:", error)
	}
}

/** Unsubscribe from real-time system updates */
export const unsubscribe = () => (unsub = unsub?.())
