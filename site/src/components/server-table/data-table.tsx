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

import { Button, buttonVariants } from '@/components/ui/button'
import { Input } from '@/components/ui/input'

import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuLabel,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'

import {
	AlertDialog,
	AlertDialogAction,
	AlertDialogCancel,
	AlertDialogContent,
	AlertDialogDescription,
	AlertDialogFooter,
	AlertDialogHeader,
	AlertDialogTitle,
} from '@/components/ui/alert-dialog'

import { SystemRecord } from '@/types'
import {
	MoreHorizontal,
	ArrowUpDown,
	Copy,
	Server,
	Cpu,
	MemoryStick,
	HardDrive,
} from 'lucide-react'
import { useMemo, useState } from 'react'
import { $servers, pb, navigate } from '@/lib/stores'
import { useStore } from '@nanostores/react'
import { AddServerButton } from '../add-server'
import { cn, copyToClipboard } from '@/lib/utils'

function CellFormatter(info: CellContext<SystemRecord, unknown>) {
	const val = info.getValue() as number
	let color = 'green'
	if (val > 80) {
		color = 'red'
	} else if (val > 50) {
		color = 'yellow'
	}
	return (
		<div className="flex gap-2 items-center">
			<span className="grow block bg-muted h-4 relative rounded-sm overflow-hidden">
				<span
					className={cn('absolute inset-0 w-full h-full origin-left', `bg-${color}-500`)}
					style={{ transform: `scalex(${val}%)` }}
				></span>
			</span>
			<span className="w-16">{val.toFixed(2)}%</span>
		</div>
	)
}

function sortableHeader(column: Column<SystemRecord, unknown>, name: string, Icon: any) {
	return (
		<Button
			variant="ghost"
			className="h-9 px-3"
			onClick={() => column.toggleSorting(column.getIsSorted() === 'asc')}
		>
			<Icon className="mr-2 h-4 w-4" />
			{name}
			<ArrowUpDown className="ml-2 h-4 w-4" />
		</Button>
	)
}

export default function () {
	const data = useStore($servers)
	const [deleteServer, setDeleteServer] = useState({} as SystemRecord)
	const [sorting, setSorting] = useState<SortingState>([])
	const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>([])

	const columns: ColumnDef<SystemRecord>[] = useMemo(
		() => [
			{
				// size: 70,
				accessorKey: 'name',
				cell: (info) => (
					<span className="flex gap-1 items-center text-base">
						<span
							className={cn(
								'w-2.5 h-2.5 block left-0 rounded-full',
								info.row.original.active ? 'bg-green-500' : 'bg-red-500'
							)}
							style={{ marginBottom: '-1px' }}
						></span>
						<Button
							variant={'ghost'}
							className="text-foreground/80 h-7 px-2 gap-1.5"
							onClick={() => copyToClipboard(info.getValue() as string)}
						>
							{info.getValue() as string}
							<Copy className="h-3.5 w-3.5 opacity-70" />
						</Button>
					</span>
				),
				header: ({ column }) => sortableHeader(column, 'Server', Server),
			},
			{
				accessorKey: 'stats.cpu',
				cell: CellFormatter,
				header: ({ column }) => sortableHeader(column, 'CPU', Cpu),
			},
			{
				accessorKey: 'stats.memPct',
				cell: CellFormatter,
				header: ({ column }) => sortableHeader(column, 'Memory', MemoryStick),
			},
			{
				accessorKey: 'stats.diskPct',
				cell: CellFormatter,
				header: ({ column }) => sortableHeader(column, 'Disk', HardDrive),
			},
			{
				id: 'actions',
				size: 32,
				maxSize: 32,
				cell: ({ row }) => {
					const system = row.original

					return (
						<div className={'flex justify-end'}>
							<DropdownMenu>
								<DropdownMenuTrigger asChild>
									<Button variant="ghost" className="h-8 w-8 p-0">
										<span className="sr-only">Open menu</span>
										<MoreHorizontal className="h-4 w-4" />
									</Button>
								</DropdownMenuTrigger>
								<DropdownMenuContent align="end">
									<DropdownMenuLabel>Actions</DropdownMenuLabel>
									<DropdownMenuItem
										onSelect={() => {
											navigate(`/server/${system.name}`)
										}}
									>
										View details
									</DropdownMenuItem>
									<DropdownMenuItem onClick={() => navigator.clipboard.writeText(system.ip)}>
										Copy IP address
									</DropdownMenuItem>
									<DropdownMenuSeparator />
									<DropdownMenuItem
										onSelect={() => {
											setDeleteServer(system)
										}}
									>
										Delete server
									</DropdownMenuItem>
								</DropdownMenuContent>
							</DropdownMenu>
						</div>
					)
				},
			},
		],
		[]
	)

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
		<>
			<div className="w-full">
				<div className="flex items-center mb-4">
					<Input
						// @ts-ignore
						placeholder="Filter..."
						value={(table.getColumn('name')?.getFilterValue() as string) ?? ''}
						onChange={(event) => table.getColumn('name')?.setFilterValue(event.target.value)}
						className="max-w-sm"
					/>
					<div className="ml-auto flex gap-2">
						<AddServerButton />
					</div>
				</div>
				<div className="rounded-md border overflow-hidden">
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
									<TableRow
										key={row.original.id}
										data-state={row.getIsSelected() && 'selected'}
										className="cursor-pointer"
										onClick={(e) => {
											const target = e.target as HTMLElement
											if (target.tagName !== 'BUTTON' && !target.hasAttribute('role')) {
												navigate(`/server/${row.original.name}`)
											}
										}}
									>
										{row.getVisibleCells().map((cell) => (
											<TableCell
												key={cell.id}
												style={{ width: `${cell.column.getSize()}px` }}
												className={'overflow-hidden relative'}
											>
												{flexRender(cell.column.columnDef.cell, cell.getContext())}
											</TableCell>
										))}
									</TableRow>
								))
							) : (
								<TableRow>
									<TableCell colSpan={columns.length} className="h-24 text-center">
										No servers found
									</TableCell>
								</TableRow>
							)}
						</TableBody>
					</Table>
				</div>
			</div>
			<AlertDialog open={!!deleteServer?.name}>
				<AlertDialogContent>
					<AlertDialogHeader>
						<AlertDialogTitle>
							Are you sure you want to delete {deleteServer.name}?
						</AlertDialogTitle>
						<AlertDialogDescription>
							This action cannot be undone. This will permanently delete all current records for{' '}
							<code className={'bg-muted rounded-sm px-1'}>{deleteServer.name}</code> from the
							database.
						</AlertDialogDescription>
					</AlertDialogHeader>
					<AlertDialogFooter>
						<AlertDialogCancel onClick={() => setDeleteServer({} as SystemRecord)}>
							Cancel
						</AlertDialogCancel>
						<AlertDialogAction
							className={cn(buttonVariants({ variant: 'destructive' }))}
							onClick={() => {
								setDeleteServer({} as SystemRecord)
								pb.collection('systems').delete(deleteServer.id)
							}}
						>
							Continue
						</AlertDialogAction>
					</AlertDialogFooter>
				</AlertDialogContent>
			</AlertDialog>
		</>
	)
}
