import {
	CellContext,
	ColumnDef,
	flexRender,
	getCoreRowModel,
	useReactTable,
} from '@tanstack/react-table'

import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from '@/components/ui/table'

import { SystemRecord } from '@/types'

function CellFormatter(info: CellContext<SystemRecord, unknown>) {
	const val = info.getValue() as number
	let background = '#42b768'
	if (val > 25) {
		background = '#da2a49'
	} else if (val > 10) {
		background = '#daa42a'
	}
	return (
		<div class="flex gap-2 items-center">
			<span>{val.toFixed(2)}%</span>
			<span class="grow block bg-secondary h-4 relative rounded-sm overflow-hidden">
				<span
					className="absolute inset-0 w-full h-full origin-left"
					style={{ transform: `scalex(${val}%)`, background }}
				></span>
			</span>
		</div>
	)
}

export function DataTable({ data }: { data: SystemRecord[] }) {
	// console.log('data', data)
	const columns: ColumnDef<SystemRecord>[] = [
		{
			header: 'Node',
			accessorKey: 'name',
		},
		{
			header: 'CPU Load',
			accessorKey: 'stats.cpu',
			cell: CellFormatter,
		},
		{
			header: 'RAM',
			accessorKey: 'stats.memPct',
			cell: CellFormatter,
		},
		{
			header: 'Disk Usage',
			accessorKey: 'stats.diskPct',
			cell: CellFormatter,
		},
	]

	const table = useReactTable({
		data,
		columns,
		getCoreRowModel: getCoreRowModel(),
	})

	return (
		<div className="rounded-md border tabular-nums">
			<Table>
				<TableHeader>
					{table.getHeaderGroups().map((headerGroup) => (
						<TableRow key={headerGroup.id}>
							{headerGroup.headers.map((header) => {
								return (
									<TableHead key={header.id}>
										{header.isPlaceholder
											? null
											: flexRender(header.column.columnDef.header, header.getContext())}
									</TableHead>
								)
							})}
						</TableRow>
					))}
				</TableHeader>
				<TableBody>
					{table.getRowModel().rows?.length ? (
						table.getRowModel().rows.map((row) => (
							<TableRow key={row.id} data-state={row.getIsSelected() && 'selected'}>
								{row.getVisibleCells().map((cell) => (
									<TableCell key={cell.id}>
										{flexRender(cell.column.columnDef.cell, cell.getContext())}
									</TableCell>
								))}
							</TableRow>
						))
					) : (
						<TableRow>
							<TableCell colSpan={columns.length} className="h-24 text-center">
								No results.
							</TableCell>
						</TableRow>
					)}
				</TableBody>
			</Table>
		</div>
	)
}
