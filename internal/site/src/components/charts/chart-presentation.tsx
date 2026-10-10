import { Trans, useLingui } from "@lingui/react/macro"
import { useStore } from "@nanostores/react"
import { useId, useState } from "react"
import { saveUserSettings } from "@/lib/api"
import { $userSettings } from "@/lib/stores"
import { $tableUpdates } from "./table-updates"

export function useChartPresentation() {
	return useStore($userSettings).chartPresentation === "tables" ? "tables" : "charts"
}
/** Account-scoped JSON setting: no schema change and no device-local fallback. */
export function ChartPresentationSelect() {
	const mode = useChartPresentation()
	const pending = useStore($tableUpdates).size > 0
	const { t } = useLingui()
	const id = useId()
	const [saving, setSaving] = useState(false)
	const [error, setError] = useState(false)
	return (
		<div className="flex flex-wrap items-center gap-2">
			<label htmlFor={id}>
				<Trans>Metric view</Trans>
			</label>
			<select
				id={id}
				value={mode}
				disabled={saving}
				className="rounded-md border bg-background p-2 text-sm focus-visible:outline-2 focus-visible:outline-ring"
				onChange={async (e) => {
					const chartPresentation = e.target.value as "tables" | "charts"
					setSaving(true)
					setError(false)
					try {
						await saveUserSettings({ chartPresentation })
					} catch {
						// Only a real API rejection raises the alert; the UI never reports a save it did not get.
						setError(true)
					} finally {
						setSaving(false)
					}
				}}
			>
				<option value="tables">{t`Tables`}</option>
				<option value="charts">{t`Charts`}</option>
			</select>
			{/* <output> maps to role=status; aria-live is spelled out for AT that ignore the implicit role. */}
			<output aria-live="polite" aria-atomic="true" className="text-sm">
				{pending ? <Trans>New data available. The table stays unchanged until you refresh it.</Trans> : ""}
			</output>
			{error && (
				<span role="alert">
					<Trans>Could not save the view. Try again.</Trans>
				</span>
			)}
		</div>
	)
}
