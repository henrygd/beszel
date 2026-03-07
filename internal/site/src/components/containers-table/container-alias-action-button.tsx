import { t } from "@lingui/core/macro"
import { PenBoxIcon } from "lucide-react"
import { type KeyboardEvent, type MouseEvent, useEffect, useState } from "react"
import { Button } from "@/components/ui/button"
import { isReadOnlyUser, pb } from "@/lib/api"
import type { ContainerRecord } from "@/types"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "../ui/dialog"
import { Input } from "../ui/input"

export function ContainerAliasActionButton({
	container,
	onAliasUpdated,
}: {
	container: ContainerRecord
	onAliasUpdated?: (containerId: string, alias: string) => void
}) {
	const [open, setOpen] = useState(false)
	const [alias, setAlias] = useState(container.alias ?? "")
	const [isSaving, setIsSaving] = useState(false)

	useEffect(() => {
		if (open) {
			setAlias(container.alias ?? "")
		}
	}, [container.alias, open])

	if (isReadOnlyUser()) {
		return null
	}

	const openDialog = (event: MouseEvent<HTMLButtonElement>) => {
		event.stopPropagation()
		setOpen(true)
	}

	const saveAlias = async () => {
		const nextAlias = alias.trim()
		if (nextAlias === (container.alias ?? "")) {
			setOpen(false)
			return
		}
		setIsSaving(true)
		try {
			await pb.collection("containers").update(container.id, { alias: nextAlias })
			onAliasUpdated?.(container.id, nextAlias)
			setOpen(false)
		} catch (error) {
			console.error(error)
		} finally {
			setIsSaving(false)
		}
	}

	const onKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
		if (event.key === "Enter") {
			event.preventDefault()
			saveAlias()
		}
	}

	return (
		<>
			<Button
				type="button"
				variant="ghost"
				size="icon"
				className="size-8"
				onClick={openDialog}
				title={t`Edit alias`}
				aria-label={t`Edit alias`}
			>
				<PenBoxIcon className="size-4" />
			</Button>
			<Dialog open={open} onOpenChange={setOpen}>
				<DialogContent className="max-w-md" onClick={(event) => event.stopPropagation()}>
					<DialogHeader>
						<DialogTitle>{t`Edit container alias`}</DialogTitle>
						<DialogDescription>{t`Choose a friendly name for this container (optional).`}</DialogDescription>
					</DialogHeader>
					<Input
						value={alias}
						onChange={(event) => setAlias(event.target.value)}
						onKeyDown={onKeyDown}
						placeholder={t`Alias (optional)`}
						autoFocus={true}
					/>
					<DialogFooter>
						<Button type="button" variant="ghost" onClick={() => setOpen(false)}>
							{t`Cancel`}
						</Button>
						<Button type="button" disabled={isSaving} onClick={saveAlias}>
							{isSaving ? t`Saving...` : t`Save`}
						</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>
		</>
	)
}
