import { memo, useMemo } from "react"
import { Row, TableType } from "@tanstack/react-table"
import { useLingui } from "@lingui/react"
import { cn } from "@/lib/utils"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Link } from "@/components/ui/link"
import { getPagePath } from "@/lib/page-path"
import { useRouter } from "next/router"
import { flexRender } from "@tanstack/react-table"
import { ColumnDef } from "@tanstack/table-core"
import { SystemRecord } from "@/lib/types"
import { IndicatorDot } from "@/components/indicator-dot"
import { AlertsButton } from "@/components/alerts-button"
import { ActionsButton } from "@/components/actions-button"
import { EyeIcon } from "@/components/icons"

const SystemCard = memo(
	({ row, table, colLength }: { row: Row<SystemRecord>; table: TableType<SystemRecord>; colLength: number }) => {
		const system = row.original
		const { t } = useLingui()

		return useMemo(() => {
			return (
				<Card
					key={system.id}
					className={cn(
						"cursor-pointer hover:shadow-md transition-all bg-transparent w-full dark:border-border duration-200 relative",
						{
							"opacity-50": system.status === "paused",
						}
					)}
				>
					<CardHeader className="py-1 ps-5 pe-3 bg-muted/30 border-b border-border/60">
						<div className="flex items-center justify-between gap-2">
							<CardTitle className="text-base tracking-normal shrink-1 text-primary/90 flex items-center min-w-0 gap-2.5">
								<div className="flex items-center gap-2.5 min-w-0">
									<IndicatorDot system={system} />
									<CardTitle className="text-[.95em]/normal tracking-normal truncate text-primary/90">
										{system.name}
									</CardTitle>
								</div>
							</CardTitle>
							{table.getColumn("actions")?.getIsVisible() && (
								<div className="flex gap-1 flex-shrink-0 relative z-10">
									<AlertsButton system={system} />
									<ActionsButton system={system} />
								</div>
							)}
						</div>
					</CardHeader>
					<CardContent className="space-y-2.5 text-sm px-5 pt-3.5 pb-4">
						{table.getAllColumns().map((column) => {
							if (!column.getIsVisible() || column.id === "system" || column.id === "actions") return null
							const cell = row.getAllCells().find((cell) => cell.column.id === column.id)
							if (!cell) return null
							// @ts-ignore
							const { Icon, name } = column.columnDef as ColumnDef<SystemRecord, unknown>

							// Special case for 'lastSeen' column: add EyeIcon before value
							if (column.id === "lastSeen") {
								return (
									<div key={column.id} className="flex items-center gap-3">
										<EyeIcon className="size-4 text-muted-foreground" />
										<div className="flex items-center gap-3 flex-1">
											<span className="text-muted-foreground min-w-16">{name()}:</span>
											<div className="flex-1">{flexRender(cell.column.columnDef.cell, cell.getContext())}</div>
										</div>
									</div>
								)
							}

							return (
								<div key={column.id} className="flex items-center gap-3">
									{Icon && <Icon className="size-4 text-muted-foreground" />}
									<div className="flex items-center gap-3 flex-1">
										<span className="text-muted-foreground min-w-16">{name()}:</span>
										<div className="flex-1">{flexRender(cell.column.columnDef.cell, cell.getContext())}</div>
									</div>
								</div>
							)
						})}
					</CardContent>
					<Link
						href={getPagePath($router, "system", { name: row.original.name })}
						className="inset-0 absolute w-full h-full"
					>
						<span className="sr-only">{row.original.name}</span>
					</Link>
				</Card>
			)
		}, [system, colLength, t])
	}
)