import {
	CellContext,
	ColumnDef,
	ColumnFiltersState,
	getFilteredRowModel,
	SortingState,
	getSortedRowModel,
	flexRender,
	getCoreRowModel,
	useReactTable,
	Column,
} from '@tanstack/react-table'

import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from '@/components/ui/table'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'

import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuLabel,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'

import { SystemRecord } from '@/types'
import { MoreHorizontal, ArrowUpDown, Copy, RefreshCcw } from 'lucide-react'
import { Link } from 'wouter-preact'
import { useState } from 'preact/hooks'

function CellFormatter(info: CellContext<SystemRecord, unknown>) {
	const val = info.getValue() as number
	let background = '#42b768'
	if (val > 80) {
		// red
		background = '#da2a49'
	} else if (val > 50) {
		// yellow
		background = '#daa42a'
	}
	return (
		<div class="flex gap-2 items-center">
			<span class="grow block bg-muted h-4 relative rounded-sm overflow-hidden">
				<span
					className="absolute inset-0 w-full h-full origin-left"
					style={{ transform: `scalex(${val}%)`, background }}
				></span>
			</span>
			<span class="w-16">{val.toFixed(2)}%</span>
		</div>
	)
}

function sortableHeader(column: Column<SystemRecord, unknown>, name: string) {
	return (
		<Button
			variant="ghost"
			className="h-9 px-3"
			onClick={() => column.toggleSorting(column.getIsSorted() === 'asc')}
		>
			{name}
			<ArrowUpDown className="ml-2 h-4 w-4" />
		</Button>
	)
}

export function DataTable({ data }: { data: SystemRecord[] }) {
	const columns: ColumnDef<SystemRecord>[] = [
		{
			// size: 70,
			accessorKey: 'name',
			cell: (info) => (
				<span className="flex gap-2 items-center text-base">
					{info.getValue() as string}{' '}
					<button
						title={`Copy "${info.getValue() as string}" to clipboard`}
						class="opacity-50 hover:opacity-70 active:opacity-100 duration-75"
						onClick={() => navigator.clipboard.writeText(info.getValue() as string)}
					>
						<Copy className="h-3.5 w-3.5 " />
					</button>
					{/* </Button> */}
				</span>
			),
			header: ({ column }) => sortableHeader(column, 'Node'),
		},
		{
			accessorKey: 'stats.cpu',
			cell: CellFormatter,
			header: ({ column }) => sortableHeader(column, 'CPU'),
		},
		{
			accessorKey: 'stats.memPct',
			cell: CellFormatter,
			header: ({ column }) => sortableHeader(column, 'Memory'),
		},
		{
			accessorKey: 'stats.diskPct',
			cell: CellFormatter,
			header: ({ column }) => sortableHeader(column, 'Disk'),
		},
		{
			id: 'actions',
			size: 32,
			maxSize: 32,
			cell: ({ row }) => {
				const system = row.original

				return (
					<div class={'flex justify-end'}>
						<DropdownMenu>
							<DropdownMenuTrigger asChild>
								<Button variant="ghost" className="h-8 w-8 p-0">
									<span className="sr-only">Open menu</span>
									<MoreHorizontal className="h-4 w-4" />
								</Button>
							</DropdownMenuTrigger>
							<DropdownMenuContent align="end">
								<DropdownMenuLabel>Actions</DropdownMenuLabel>
								<DropdownMenuItem>
									<Link class="w-full" href={`/server/${system.name}`}>
										View details
									</Link>
								</DropdownMenuItem>
								<DropdownMenuItem onClick={() => navigator.clipboard.writeText(system.id)}>
									Copy IP address
								</DropdownMenuItem>
								<DropdownMenuSeparator />
								<DropdownMenuItem>Delete node</DropdownMenuItem>
							</DropdownMenuContent>
						</DropdownMenu>
					</div>
				)
			},
		},
	]

	const [sorting, setSorting] = useState<SortingState>([])

	const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>([])

	const table = useReactTable({
		data,
		columns,
		getCoreRowModel: getCoreRowModel(),
		onSortingChange: setSorting,
		getSortedRowModel: getSortedRowModel(),
		onColumnFiltersChange: setColumnFilters,
		getFilteredRowModel: getFilteredRowModel(),
		state: {
			sorting,
			columnFilters,
		},
	})

	return (
		<div className="w-full">
			<div className="flex items-center mb-4">
				<Input
					// @ts-ignore
					placeholder="Filter..."
					value={(table.getColumn('name')?.getFilterValue() as string) ?? ''}
					onChange={(event: Event) => table.getColumn('name')?.setFilterValue(event.target.value)}
					className="max-w-sm"
				/>
				<Button
					variant="outline"
					onClick={() => {
						alert('todo: refresh')
					}}
					className="ml-auto flex gap-2"
				>
					<RefreshCcw className="h-4 w-4" />
					Refresh
				</Button>
			</div>
			<div className="rounded-md border">
				<Table>
					<TableHeader className="bg-muted/40">
						{table.getHeaderGroups().map((headerGroup) => (
							<TableRow key={headerGroup.id}>
								{headerGroup.headers.map((header) => {
									return (
										<TableHead className="px-2" key={header.id}>
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
										<TableCell key={cell.id} style={{ width: `${cell.column.getSize()}px` }}>
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
		</div>
	)
}
