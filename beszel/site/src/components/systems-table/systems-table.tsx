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
	AlertDialogTrigger,
} from '@/components/ui/alert-dialog'

import { SystemRecord } from '@/types'
import {
	MoreHorizontal,
	ArrowUpDown,
	Server,
	Cpu,
	MemoryStick,
	HardDrive,
	CopyIcon,
	PauseCircleIcon,
	PlayCircleIcon,
	Trash2Icon,
} from 'lucide-react'
import { useMemo, useState } from 'react'
import { $systems, pb } from '@/lib/stores'
import { useStore } from '@nanostores/react'
import { AddSystemButton } from '../add-system'
import { cn, copyToClipboard, isReadOnlyUser } from '@/lib/utils'
import AlertsButton from '../table-alerts'
import { navigate } from '../router'

function CellFormatter(info: CellContext<SystemRecord, unknown>) {
	const val = info.getValue() as number
	return (
		<div className="flex gap-1 items-center tabular-nums tracking-tight">
			<span className="min-w-[3.5em]">{val.toFixed(1)}%</span>
			<span className="grow min-w-10 block bg-muted h-[1em] relative rounded-sm overflow-hidden">
				<span
					className={cn(
						'absolute inset-0 w-full h-full origin-left',
						(val < 60 && 'bg-green-500') || (val < 90 && 'bg-yellow-500') || 'bg-red-600'
					)}
					style={{ transform: `scalex(${val}%)` }}
				></span>
			</span>
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

export default function SystemsTable() {
	const data = useStore($systems)
	const [sorting, setSorting] = useState<SortingState>([])
	const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>([])

	const columns: ColumnDef<SystemRecord>[] = useMemo(() => {
		return [
			{
				// size: 200,
				size: 200,
				minSize: 0,
				accessorKey: 'name',
				cell: (info) => {
					const { status } = info.row.original
					return (
						<span className="flex gap-0.5 items-center text-base md:pr-5">
							<span
								className={cn('w-2 h-2 left-0 rounded-full', {
									'bg-green-500': status === 'up',
									'bg-red-500': status === 'down',
									'bg-primary/40': status === 'paused',
									'bg-yellow-500': status === 'pending',
								})}
								style={{ marginBottom: '-1px' }}
							></span>
							<Button
								data-nolink
								variant={'ghost'}
								className="text-foreground/90 h-7 px-1.5 gap-1.5"
								onClick={() => copyToClipboard(info.getValue() as string)}
							>
								{info.getValue() as string}
								<CopyIcon className="h-2.5 w-2.5" />
							</Button>
						</span>
					)
				},
				header: ({ column }) => sortableHeader(column, 'System', Server),
			},
			{
				accessorKey: 'info.cpu',
				cell: CellFormatter,
				header: ({ column }) => sortableHeader(column, 'CPU', Cpu),
			},
			{
				accessorKey: 'info.mp',
				cell: CellFormatter,
				header: ({ column }) => sortableHeader(column, 'Memory', MemoryStick),
			},
			{
				accessorKey: 'info.dp',
				cell: CellFormatter,
				header: ({ column }) => sortableHeader(column, 'Disk', HardDrive),
			},
			{
				id: 'actions',
				size: 120,
				// minSize: 0,
				cell: ({ row }) => {
					const { id, name, status, host } = row.original
					return (
						<div className={'flex justify-end items-center gap-1'}>
							<AlertsButton system={row.original} />
							<AlertDialog>
								<DropdownMenu>
									<DropdownMenuTrigger asChild>
										<Button variant="ghost" size={'icon'} data-nolink>
											<span className="sr-only">Open menu</span>
											<MoreHorizontal className="w-5" />
										</Button>
									</DropdownMenuTrigger>
									<DropdownMenuContent align="end">
										<DropdownMenuItem
											className={cn(isReadOnlyUser() && 'hidden')}
											onClick={() => {
												pb.collection('systems').update(id, {
													status: status === 'paused' ? 'pending' : 'paused',
												})
											}}
										>
											{status === 'paused' ? (
												<>
													<PlayCircleIcon className="mr-2.5 h-4 w-4" />
													Resume
												</>
											) : (
												<>
													<PauseCircleIcon className="mr-2.5 h-4 w-4" />
													Pause
												</>
											)}
										</DropdownMenuItem>
										<DropdownMenuItem onClick={() => copyToClipboard(host)}>
											<CopyIcon className="mr-2.5 h-4 w-4" />
											Copy host
										</DropdownMenuItem>
										<DropdownMenuSeparator className={cn(isReadOnlyUser() && 'hidden')} />
										<AlertDialogTrigger asChild>
											<DropdownMenuItem className={cn(isReadOnlyUser() && 'hidden')}>
												<Trash2Icon className="mr-2.5 h-4 w-4" />
												Delete
											</DropdownMenuItem>
										</AlertDialogTrigger>
									</DropdownMenuContent>
								</DropdownMenu>
								<AlertDialogContent>
									<AlertDialogHeader>
										<AlertDialogTitle>Are you sure you want to delete {name}?</AlertDialogTitle>
										<AlertDialogDescription>
											This action cannot be undone. This will permanently delete all current records
											for <code className={'bg-muted rounded-sm px-1'}>{name}</code> from the
											database.
										</AlertDialogDescription>
									</AlertDialogHeader>
									<AlertDialogFooter>
										<AlertDialogCancel>Cancel</AlertDialogCancel>
										<AlertDialogAction
											className={cn(buttonVariants({ variant: 'destructive' }))}
											onClick={() => pb.collection('systems').delete(id)}
										>
											Continue
										</AlertDialogAction>
									</AlertDialogFooter>
								</AlertDialogContent>
							</AlertDialog>
						</div>
					)
				},
			},
		]
	}, [])

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
		defaultColumn: {
			minSize: 0,
			size: Number.MAX_SAFE_INTEGER,
			maxSize: Number.MAX_SAFE_INTEGER,
		},
	})

	return (
		<>
			<div className="w-full">
				<div className="flex items-center mb-4 gap-2">
					<Input
						// @ts-ignore
						placeholder="Filter..."
						value={(table.getColumn('name')?.getFilterValue() as string) ?? ''}
						onChange={(event) => table.getColumn('name')?.setFilterValue(event.target.value)}
						className="max-w-sm"
					/>
					<div className={cn('ml-auto flex gap-2', isReadOnlyUser() && 'hidden')}>
						<AddSystemButton />
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
										className={cn('cursor-pointer transition-opacity', {
											'opacity-50': row.original.status === 'paused',
										})}
										onClick={(e) => {
											const target = e.target as HTMLElement
											if (!target.closest('[data-nolink]') && e.currentTarget.contains(target)) {
												navigate(`/system/${encodeURIComponent(row.original.name)}`)
											}
										}}
									>
										{row.getVisibleCells().map((cell) => (
											<TableCell
												key={cell.id}
												style={{
													width:
														cell.column.getSize() === Number.MAX_SAFE_INTEGER
															? 'auto'
															: cell.column.getSize(),
												}}
												className={'overflow-hidden relative py-2.5'}
											>
												{flexRender(cell.column.columnDef.cell, cell.getContext())}
											</TableCell>
										))}
									</TableRow>
								))
							) : (
								<TableRow>
									<TableCell colSpan={columns.length} className="h-24 text-center">
										No systems found
									</TableCell>
								</TableRow>
							)}
						</TableBody>
					</Table>
				</div>
			</div>
		</>
	)
}
