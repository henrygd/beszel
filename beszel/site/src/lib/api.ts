import { ChartTimes, SystemRecord, UserSettings } from "@/types"
import { $alerts, $systems, $userSettings } from "./stores"
import { toast } from "@/components/ui/use-toast"
import { t } from "@lingui/core/macro"
import { chartTimeData } from "./utils"
import { WritableAtom } from "nanostores"
import { RecordModel, RecordSubscription } from "pocketbase"
import PocketBase from "pocketbase"
import { basePath } from "@/components/router"

/** PocketBase JS Client */
export const pb = new PocketBase(basePath)

export const isAdmin = () => pb.authStore.record?.role === "admin"
export const isReadOnlyUser = () => pb.authStore.record?.role === "readonly"

const verifyAuth = () => {
	pb.collection("users")
		.authRefresh()
		.catch(() => {
			logOut()
			toast({
				title: t`Failed to authenticate`,
				description: t`Please log in again`,
				variant: "destructive",
			})
		})
}

/** Logs the user out by clearing the auth store and unsubscribing from realtime updates. */
export async function logOut() {
	$systems.set([])
	$alerts.set({})
	$userSettings.set({} as UserSettings)
	sessionStorage.setItem("lo", "t") // prevent auto login on logout
	pb.authStore.clear()
	pb.realtime.unsubscribe()
}

/** Fetch or create user settings in database */
export async function updateUserSettings() {
	try {
		const req = await pb.collection("user_settings").getFirstListItem("", { fields: "settings" })
		$userSettings.set(req.settings)
		return
	} catch (e) {
		console.error("get settings", e)
	}
	// create user settings if error fetching existing
	try {
		const createdSettings = await pb.collection("user_settings").create({ user: pb.authStore.record!.id })
		$userSettings.set(createdSettings.settings)
	} catch (e) {
		console.error("create settings", e)
	}
}
/** Update systems / alerts list when records change  */
export function updateRecordList<T extends RecordModel>(e: RecordSubscription<T>, $store: WritableAtom<T[]>) {
	const curRecords = $store.get()
	const newRecords = []
	if (e.action === "delete") {
		for (const server of curRecords) {
			if (server.id !== e.record.id) {
				newRecords.push(server)
			}
		}
	} else {
		let found = 0
		for (const server of curRecords) {
			if (server.id === e.record.id) {
				found = newRecords.push(e.record)
			} else {
				newRecords.push(server)
			}
		}
		if (!found) {
			newRecords.push(e.record)
		}
	}
	$store.set(newRecords)
}
/** Fetches updated system list from database */
export const updateSystemList = (() => {
	let isFetchingSystems = false
	return async () => {
		if (isFetchingSystems) {
			return
		}
		isFetchingSystems = true
		try {
			const records = await pb
				.collection<SystemRecord>("systems")
				.getFullList({ sort: "+name", fields: "id,name,host,port,info,status" })

			if (records.length) {
				$systems.set(records)
			} else {
				verifyAuth()
			}
		} finally {
			isFetchingSystems = false
		}
	}
})()

export function getPbTimestamp(timeString: ChartTimes, d?: Date) {
	d ||= chartTimeData[timeString].getOffset(new Date())
	const year = d.getUTCFullYear()
	const month = String(d.getUTCMonth() + 1).padStart(2, "0")
	const day = String(d.getUTCDate()).padStart(2, "0")
	const hours = String(d.getUTCHours()).padStart(2, "0")
	const minutes = String(d.getUTCMinutes()).padStart(2, "0")
	const seconds = String(d.getUTCSeconds()).padStart(2, "0")

	return `${year}-${month}-${day} ${hours}:${minutes}:${seconds}`
}
