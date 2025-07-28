import { Trans } from "@lingui/react/macro"
import { useStore } from "@nanostores/react"
import { $alerts, $systems, pb } from "@/lib/stores"
import { alertInfo } from "@/lib/utils"
import { Separator } from "@/components/ui/separator"
import { Button } from "@/components/ui/button"
import type { AlertRecord, SystemRecord } from "@/types"
import React from "react"
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetDescription,
} from "@/components/ui/sheet"
import { toast } from "@/components/ui/use-toast"
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuItem,
} from "@/components/ui/dropdown-menu";
import { updateAlertsForSystems } from "@/components/alerts/alerts-system"
import { PlusIcon } from "lucide-react"
import MultiSystemAlertSheetContent from "@/components/alerts/alerts-multi-sheet"
import { useReactTable, getCoreRowModel, flexRender, getPaginationRowModel, getSortedRowModel, getFilteredRowModel, SortingState, ColumnFiltersState, VisibilityState } from "@tanstack/react-table"
import { Table, TableHeader, TableBody, TableHead, TableRow, TableCell } from "@/components/ui/table"
import { Checkbox } from "@/components/ui/checkbox"
import { Input } from "@/components/ui/input"
import { MoreHorizontal } from "lucide-react"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Label } from "@/components/ui/label"
import { PenBoxIcon, Trash2Icon, ServerIcon, ClockIcon, OctagonAlert, TagIcon, ArrowUpDownIcon, ChevronLeftIcon, ChevronRightIcon, ChevronsLeftIcon, ChevronsRightIcon } from "lucide-react"
import { updateAlerts } from "@/lib/utils"


// Reusable DataTable component
function DataTable<TData extends { alerts: { id: string }[] }>({
  table,
  columnsLength,
  onBulkDelete,
}: {
  table: ReturnType<typeof useReactTable<TData>>,
  columnsLength: number,
  onBulkDelete?: () => void,
}) {
  const pageSizes = [5, 10, 15, 20];
  return (
    <div>
      <div className="rounded-md border">
        <Table>
          <TableHeader>
            {table.getHeaderGroups().map((headerGroup) => (
              <TableRow key={headerGroup.id}>
                {headerGroup.headers.map((header) => (
                  <TableHead key={header.id}>
                    {header.isPlaceholder
                      ? null
                      : flexRender(
                          header.column.columnDef.header,
                          header.getContext()
                        )}
                  </TableHead>
                ))}
              </TableRow>
            ))}
          </TableHeader>
          <TableBody>
            {table.getRowModel().rows?.length ? (
              table.getRowModel().rows.map((row) => (
                <TableRow
                  key={row.id}
                  data-state={row.getIsSelected() && "selected"}
                >
                  {row.getVisibleCells().map((cell) => (
                    <TableCell key={cell.id}>
                      {flexRender(
                        cell.column.columnDef.cell,
                        cell.getContext()
                      )}
                    </TableCell>
                  ))}
                </TableRow>
              ))
            ) : (
              <TableRow>
                <TableCell
                  colSpan={columnsLength}
                  className="h-24 text-center"
                >
                  No results.
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </div>
      <div className="flex items-center justify-between px-4 py-4">
        <div className="text-muted-foreground hidden flex-1 text-sm lg:flex">
          {table.getFilteredSelectedRowModel().rows.length} of {table.getFilteredRowModel().rows.length} row(s) selected.
        </div>
        <div className="flex w-full items-center gap-8 lg:w-fit">
          <div className="hidden items-center gap-2 lg:flex">
            <Label htmlFor="rows-per-page" className="text-sm font-medium">
              Rows per page
            </Label>
            <Select
              value={`${table.getState().pagination.pageSize}`}
              onValueChange={(value) => {
                table.setPageSize(Number(value))
              }}
            >
              <SelectTrigger className="w-20" id="rows-per-page">
                <SelectValue
                  placeholder={table.getState().pagination.pageSize}
                />
              </SelectTrigger>
              <SelectContent side="top">
                {pageSizes.map((pageSize) => (
                  <SelectItem key={pageSize} value={`${pageSize}`}>
                    {pageSize}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="flex w-fit items-center justify-center text-sm font-medium">
            Page {table.getState().pagination.pageIndex + 1} of {table.getPageCount()}
          </div>
          <div className="ml-auto flex items-center gap-2 lg:ml-0">
            <Button
              variant="outline"
              className="hidden h-8 w-8 p-0 lg:flex"
              onClick={() => table.setPageIndex(0)}
              disabled={!table.getCanPreviousPage()}
            >
              <span className="sr-only">Go to first page</span>
              <ChevronsLeftIcon className="h-4 w-4" />
            </Button>
            <Button
              variant="outline"
              className="size-8"
              size="icon"
              onClick={() => table.previousPage()}
              disabled={!table.getCanPreviousPage()}
            >
              <span className="sr-only">Go to previous page</span>
              <ChevronLeftIcon className="h-4 w-4" />
            </Button>
            <Button
              variant="outline"
              className="size-8"
              size="icon"
              onClick={() => table.nextPage()}
              disabled={!table.getCanNextPage()}
            >
              <span className="sr-only">Go to next page</span>
              <ChevronRightIcon className="h-4 w-4" />
            </Button>
            <Button
              variant="outline"
              className="hidden size-8 lg:flex"
              size="icon"
              onClick={() => table.setPageIndex(table.getPageCount() - 1)}
              disabled={!table.getCanNextPage()}
            >
              <span className="sr-only">Go to last page</span>
              <ChevronsRightIcon className="h-4 w-4" />
            </Button>
          </div>
        </div>
      </div>
    </div>
  )
}

function ConfiguredAlertsTab({ alerts, systems, onAddAlert }: { alerts: AlertRecord[]; systems: SystemRecord[]; onAddAlert: () => void }) {
	// Group alerts by type (name)
	const alertsByType: Record<string, AlertRecord[]> = React.useMemo(() => {
		const map: Record<string, AlertRecord[]> = {};
		for (const alert of alerts) {
			if (!map[alert.name]) map[alert.name] = [];
			map[alert.name].push(alert);
		}
		return map;
	}, [alerts]);

	// Sheet state for editing a group of alerts
	const [openSheet, setOpenSheet] = React.useState(false)
	const [editAlerts, setEditAlerts] = React.useState<AlertRecord[] | null>(null)
	const [editValue, setEditValue] = React.useState<number | null>(null)
	const [editMin, setEditMin] = React.useState<number | null>(null)

	React.useEffect(() => {
		if (editAlerts && openSheet) {
			setEditValue(editAlerts[0]?.value ?? null)
			setEditMin(editAlerts[0]?.min ?? null)
		}
	}, [editAlerts, openSheet])

	function openEditSheet(alerts: AlertRecord[]) {
		setEditAlerts(alerts)
		setOpenSheet(true)
	}

	function closeSheet() {
		setOpenSheet(false)
		setEditAlerts(null)
		setEditValue(null)
		setEditMin(null)
	}

	async function handleSave(alerts: AlertRecord[]) {
		if (!editValue || !editMin) return;
		const systemsToUpdate = systems.filter(s => alerts.some(a => a.system === s.id));
		const alertsBySystem = new Map(alerts.map(a => [a.system, a]));
		await updateAlertsForSystems({
			systems: systemsToUpdate,
			alertsBySystem,
			alertName: alerts[0].name,
			checked: true,
			value: editValue,
			min: editMin,
			userId: pb.authStore.model?.id || pb.authStore.record?.id || "",
			onAllDisabled: undefined,
			systemAlerts: alerts,
			allSystems: systems,
		})
		toast({
			title: "Alert updated",
			description: "Your alert configuration has been saved.",
		})
		closeSheet()
	}

	async function handleDelete(alerts: AlertRecord[]) {
		const confirmed = window.confirm(`Are you sure you want to delete this alert for all selected systems? This action cannot be undone.`);
		if (!confirmed) return;
		for (const alert of alerts) {
			await pb.collection("alerts").delete(alert.id);
		}
		toast({
			title: "Alert deleted",
			description: "Your alert configuration has been deleted.",
		});
		closeSheet();
	}

	// --- FLATTENED DATA FOR TABLE ---

type AlertTableRow = {
	type: string;
	systemNames: string;
	min: number;
	value: number;
	alerts: AlertRecord[];
};

const tableData: AlertTableRow[] = React.useMemo(() => {
	const rows: AlertTableRow[] = [];
	for (const type of Object.keys(alertsByType)) {
		const groupMap: Record<string, AlertRecord[]> = {};
		for (const alert of alertsByType[type]) {
			const key = `${alert.min}|${alert.value}`;
			if (!groupMap[key]) groupMap[key] = [];
			groupMap[key].push(alert);
		}
		for (const [key, groupAlerts] of Object.entries(groupMap)) {
			const min = groupAlerts[0].min;
			const value = groupAlerts[0].value;
			const systemNames = groupAlerts
				.map(alert => systems.find(s => s.id === alert.system)?.name ?? alert.system)
				.join(', ');
			rows.push({
				type: alertInfo[type]?.name() ?? type,
				systemNames,
				min,
				value,
				alerts: groupAlerts,
			});
		}
	}
	return rows;
}, [alertsByType, systems]);

// --- TABLE STATE ---
const [sorting, setSorting] = React.useState<SortingState>([])
const [columnFilters, setColumnFilters] = React.useState<ColumnFiltersState>([])
const [columnVisibility, setColumnVisibility] = React.useState<VisibilityState>({})
const [rowSelection, setRowSelection] = React.useState({})
const [combinedFilter, setCombinedFilter] = React.useState("");
const [pagination, setPagination] = React.useState({ pageIndex: 0, pageSize: 5 });

// --- TABLE COLUMNS ---
const columns = React.useMemo<import("@tanstack/react-table").ColumnDef<AlertTableRow, any>[]>(() => [
	{
		id: "select",
		enableSorting: false,
		enableHiding: false,
		header: ({ table }) => (
			<Checkbox
				checked={
					table.getIsAllPageRowsSelected() ||
					(table.getIsSomePageRowsSelected() && "indeterminate")
				}
				onCheckedChange={(value) => table.toggleAllPageRowsSelected(!!value)}
				aria-label="Select all"
			/>
		),
		cell: ({ row }) => (
			<Checkbox
				checked={row.getIsSelected()}
				onCheckedChange={(value) => row.toggleSelected(!!value)}
				aria-label="Select row"
			/>
		),
	},
	{ 
		accessorKey: "type", 
		name: () => `Type`,
		Icon: TagIcon,
		header: ({ column }) => (
			<Button
				variant="ghost"
				className="justify-center w-full"
				onClick={() => column.toggleSorting(column.getIsSorted() === "asc")}
			>
				<TagIcon className="mr-2 h-4 w-4 opacity-70" />
				Type <ArrowUpDownIcon className="ml-2 h-3 w-3" />
			</Button>
		),
		cell: info => <span className="text-center block">{info.getValue()}</span>,
	},
	{
		accessorKey: "systemNames",
		name: () => `System`,
		Icon: ServerIcon,
		header: ({ column }) => (
			<Button
				variant="ghost"
				className="justify-center w-full"
				onClick={() => column.toggleSorting(column.getIsSorted() === "asc")}
			>
				<ServerIcon className="mr-2 h-4 w-4 opacity-70" />
				System <ArrowUpDownIcon className="ml-2 h-3 w-3" />
			</Button>
		),
		cell: info => <span className="text-center block">{info.getValue()}</span>,
		filterFn: (row, columnId, filterValue) => {
			const systemNamesRaw = row.getValue(columnId);
			const systemNames = typeof systemNamesRaw === "string" ? systemNamesRaw.toLowerCase() : "";
			const type = row.original.type?.toLowerCase() ?? "";
			const value = (filterValue || "").toLowerCase();
			return systemNames.includes(value) || type.includes(value);
		},
	},
	{ 
		accessorKey: "value",
		name: () => `Value`,
		Icon: OctagonAlert,
		header: ({ column }) => (
			<Button
				variant="ghost"
				className="justify-center w-full"
				onClick={() => column.toggleSorting(column.getIsSorted() === "asc")}
			>
				<OctagonAlert className="mr-2 h-4 w-4 opacity-70" />
				Value <ArrowUpDownIcon className="ml-2 h-3 w-3" />
			</Button>
		),
		cell: info => <span className="text-center block">{info.getValue()}</span>,
	},
	{ 
		accessorKey: "min",
		name: () => `Duration`,
		Icon: ClockIcon,
		header: ({ column }) => (
			<Button
				variant="ghost"
				className="justify-center w-full"
				onClick={() => column.toggleSorting(column.getIsSorted() === "asc")}
			>
				<ClockIcon className="mr-2 h-4 w-4 opacity-70" />
				Duration <ArrowUpDownIcon className="ml-2 h-3 w-3" />
			</Button>
		),
		cell: info => <span className="text-center block">{info.getValue()}</span>,
	},
	{
		id: "actions",
		enableHiding: false,
		cell: ({ row }) => {
			const groupAlerts = row.original.alerts
			return (
				<DropdownMenu>
					<DropdownMenuTrigger asChild>
						<Button variant="ghost" className="h-8 w-8 p-0">
							<span className="sr-only">Open menu</span>
							<MoreHorizontal />
						</Button>
					</DropdownMenuTrigger>
					<DropdownMenuContent align="end">
						<DropdownMenuLabel>Actions</DropdownMenuLabel>
						<DropdownMenuItem onClick={() => openEditSheet(groupAlerts)}>
							<PenBoxIcon className="me-2.5 size-4" />
							<Trans>Edit</Trans>
						</DropdownMenuItem>
						<DropdownMenuItem onClick={() => handleDelete(groupAlerts)}>
							<Trash2Icon className="me-2.5 size-4" />
							<Trans>Delete</Trans>
						</DropdownMenuItem>
					</DropdownMenuContent>
				</DropdownMenu>
			)
		},
	},
], [openEditSheet, handleDelete]);

const table = useReactTable({
	data: tableData,
	columns,
	onSortingChange: setSorting,
	onColumnFiltersChange: setColumnFilters,
	getCoreRowModel: getCoreRowModel(),
	getPaginationRowModel: getPaginationRowModel(),
	getSortedRowModel: getSortedRowModel(),
	getFilteredRowModel: getFilteredRowModel(),
	onColumnVisibilityChange: setColumnVisibility,
	onRowSelectionChange: setRowSelection,
	state: {
		sorting,
		columnFilters,
		columnVisibility,
		rowSelection,
		pagination,
	},
	onPaginationChange: setPagination,
});

// Bulk delete logic
const selectedRows = table.getFilteredSelectedRowModel().rows;
const selectedAlertIds = selectedRows.flatMap(row => row.original.alerts.map(a => a.id));
async function handleBulkDelete() {
	if (!selectedAlertIds.length) return;
	if (!window.confirm(`Delete ${selectedAlertIds.length} selected alerts?`)) return;
	for (const id of selectedAlertIds) {
		await pb.collection("alerts").delete(id);
	}
	await updateAlerts();
	table.resetRowSelection();
}

return (
	<div className="w-full">
		<div className="flex items-center py-4 gap-2">
			<Input
				placeholder="Filter systems or type..."
				value={combinedFilter}
				onChange={event => {
					setCombinedFilter(event.target.value);
					table.getColumn("systemNames")?.setFilterValue(event.target.value);
				}}
				className="max-w-sm"
			/>
		<div className="flex gap-2 items-center ml-4">
          <Button
            variant="destructive"
            size="sm"
            disabled={selectedAlertIds.length === 0}
            onClick={handleBulkDelete}
          >
            Delete Selected
          </Button>
        </div>
			<div className="flex items-center gap-2 ml-auto">
				<Button variant="outline" onClick={onAddAlert}>
					<PlusIcon className="w-4 h-4 mr-2" />
					<Trans>Add Alert</Trans>
				</Button>
			</div>
		</div>
		<DataTable table={table} columnsLength={columns.length} onBulkDelete={handleBulkDelete} />
		{/* Single Sheet instance for editing */}
		<Sheet open={openSheet} onOpenChange={open => open ? setOpenSheet(true) : closeSheet()}>
			{editAlerts ? (
				<SheetContent side="right" className="flex flex-col h-full">
					<SheetHeader>
						<SheetTitle><Trans>Edit Alert</Trans></SheetTitle>
						<SheetDescription>Edit the alert configuration for these systems.</SheetDescription>
					</SheetHeader>
					<MultiSystemAlertSheetContent
						systems={systems}
						alerts={alerts}
						initialSystems={editAlerts.map(a => a.system)}
						onClose={closeSheet}
						singleAlertType={editAlerts[0]?.name}
						value={editValue ?? undefined}
						min={editMin ?? undefined}
						onValueChange={setEditValue}
						onMinChange={setEditMin}
					/>
				</SheetContent>
			) : (
				openSheet && closeSheet(), null
			)}
		</Sheet>
	</div>
)
}

export default function AlertsSettingsPage() {
	const alerts = useStore($alerts)
	const systems = useStore($systems)

	// Add Alert Sheet state
	const [addSheetOpen, setAddSheetOpen] = React.useState(false)
	const [addAlertSystems] = React.useState<string[]>([]);

	return (
		<div>
			<div className="flex items-center justify-between mb-4">
				<h3 className="text-xl font-medium">
					<Trans>Alerts</Trans>
				</h3>
			</div>
			<p className="text-sm text-muted-foreground leading-relaxed mb-4">
				<Trans>Overview of all configured alerts, grouped by alert type and configuration.</Trans>
			</p>
			<Separator className="my-4" />
			<ConfiguredAlertsTab alerts={alerts} systems={systems} onAddAlert={() => setAddSheetOpen(true)} />

			{/* Add Alert Sheet */}
			<Sheet open={addSheetOpen} onOpenChange={setAddSheetOpen}>
				<SheetContent side="right" className="flex flex-col h-full w-[55em] max-w-3xl">
					<SheetHeader>
						<SheetTitle><Trans>Add Alert</Trans></SheetTitle>
						<SheetDescription>
							<Trans>Select systems and configure alerts below.</Trans>
						</SheetDescription>
					</SheetHeader>
					<MultiSystemAlertSheetContent
						systems={systems}
						alerts={[]}
						initialSystems={addAlertSystems}
						onClose={() => setAddSheetOpen(false)}
						hideSystemSelector={false}
					/>
				</SheetContent>
			</Sheet>
		</div>
	)
}