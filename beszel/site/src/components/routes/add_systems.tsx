import { Suspense, lazy, useEffect } from "react"
import { $hubVersion, $newSystems, pb } from "@/lib/stores"
import { useStore } from "@nanostores/react"
import { GithubIcon } from "lucide-react"
import { Separator } from "../ui/separator"
import { updateRecordList, updateNewSystemsList } from "@/lib/utils"
import { AddSystemRecord } from "@/types"
import { t } from "@lingui/macro"

const NewSystemsTable = lazy(() => import("../systems-table/new-systems-table"))

export default function AddSystems() {
	const hubVersion = useStore($hubVersion)

	const newSystems = useStore($newSystems)

	useEffect(() => {
		document.title = t`Add System` + " / Beszel"

		// make sure we have the latest list of pending systems registrations
		updateNewSystemsList()

		// subscribe to real time updates for systems / alerts
		pb.collection<AddSystemRecord>("new_systems").subscribe("*", (e) => {
			updateRecordList(e, $newSystems)
		})
		return () => {
			pb.collection("systems").unsubscribe("*")
		}
	}, [])

	return (
		<>
			{/* show active alerts */}
			<Suspense>
				<NewSystemsTable />
			</Suspense>

			{hubVersion && (
				<div className="flex gap-1.5 justify-end items-center pe-3 sm:pe-6 mt-3.5 text-xs opacity-80">
					<a
						href="https://github.com/henrygd/beszel"
						target="_blank"
						className="flex items-center gap-0.5 text-muted-foreground hover:text-foreground duration-75"
					>
						<GithubIcon className="h-3 w-3" /> GitHub
					</a>
					<Separator orientation="vertical" className="h-2.5 bg-muted-foreground opacity-70" />
					<a
						href="https://github.com/henrygd/beszel/releases"
						target="_blank"
						className="text-muted-foreground hover:text-foreground duration-75"
					>
						Beszel {hubVersion}
					</a>
				</div>
			)}
		</>
	)
}
