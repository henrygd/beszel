import type { ReactNode } from "react"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"

/** One titled table of the hardware inventory; renders nothing when there are no rows. */
export function HardwareSection({ title, head, rows }: { title: string; head: string[]; rows: ReactNode[][] }) {
	if (rows.length === 0) return null
	return (
		<div className="grid gap-2">
			<h3 className="px-2 sm:px-1 text-sm font-medium">{title}</h3>
			<div className="rounded-md border overflow-x-auto">
				<Table>
					<TableHeader>
						<TableRow>
							{head.map((h) => (
								<TableHead key={h} className="px-3">
									{h}
								</TableHead>
							))}
						</TableRow>
					</TableHeader>
					<TableBody>
						{rows.map((row, i) => (
							<TableRow key={i}>
								{row.map((cell, j) => (
									<TableCell key={j} className="px-3 py-2 whitespace-nowrap">
										{cell ?? "-"}
									</TableCell>
								))}
							</TableRow>
						))}
					</TableBody>
				</Table>
			</div>
		</div>
	)
}
