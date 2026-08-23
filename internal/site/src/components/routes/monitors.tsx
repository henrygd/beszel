import { Trans, useLingui } from "@lingui/react/macro"
import { memo, Suspense, useEffect, useMemo, useState } from "react"
import { PlusIcon } from "lucide-react"
import MonitorsTable from "@/components/monitors-table/monitors-table"
import { AddMonitorDialog } from "@/components/add-monitor"
import { Button } from "@/components/ui/button"
import { isReadOnlyUser } from "@/lib/api"
import { FooterRepoLink } from "@/components/footer-repo-link"

export default memo(() => {
	const { t } = useLingui()
	const [dialogOpen, setDialogOpen] = useState(false)

	useEffect(() => {
		document.title = `${t`Monitors`} / Beszel`
	}, [t])

	return useMemo(
		() => (
			<>
				<AddMonitorDialog open={dialogOpen} setOpen={setDialogOpen} />
				<div className="flex flex-col gap-4">
					{!isReadOnlyUser() && (
						<div className="flex justify-end">
							<Button onClick={() => setDialogOpen(true)} className="flex gap-1">
								<PlusIcon className="h-4 w-4 -ms-1" />
								<Trans>Add Monitor</Trans>
							</Button>
						</div>
					)}
					<Suspense>
						<MonitorsTable />
					</Suspense>
				</div>
				<FooterRepoLink />
			</>
		),
		[]
	)
})
