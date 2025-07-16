"use client"

import * as React from "react"
import { useStore } from "@nanostores/react"
import { $alertsHistory, pb } from "@/lib/stores"
import { AlertsHistoryRecord } from "@/types"
import {
  getCoreRowModel,
  getPaginationRowModel,
  getSortedRowModel,
  getFilteredRowModel,
  useReactTable,
  flexRender,
  ColumnFiltersState,
  SortingState,
  VisibilityState,
} from "@tanstack/react-table"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { alertsHistoryColumns } from "../../alerts-history-columns"
import { ChevronDown } from "lucide-react"
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Checkbox } from "@/components/ui/checkbox"
import { toast } from "sonner"

export default function AlertsHistoryDataTable() {
  const alertsHistory = useStore($alertsHistory)

  React.useEffect(() => {
    pb.collection<AlertsHistoryRecord>("alerts_history")
      .getFullList({
        sort: "-created_date",
        expand: "system,user,alert"
      })
      .then(records => {
        $alertsHistory.set(records)
      })
  }, [])

  const [sorting, setSorting] = React.useState<SortingState>([])
  const [columnFilters, setColumnFilters] = React.useState<ColumnFiltersState>([])
  const [columnVisibility, setColumnVisibility] = React.useState<VisibilityState>({})
  const [rowSelection, setRowSelection] = React.useState({})
  const [combinedFilter, setCombinedFilter] = React.useState("")
  const [globalFilter, setGlobalFilter] = React.useState("")

  const table = useReactTable({
    data: alertsHistory,
    columns: [
      {
        id: "select",
        header: ({ table }) => (
          <Checkbox
            checked={
              table.getIsAllPageRowsSelected() ||
              (table.getIsSomePageRowsSelected() && "indeterminate")
            }
            onCheckedChange={value => table.toggleAllPageRowsSelected(!!value)}
            aria-label="Select all"
          />
        ),
        cell: ({ row }) => (
          <Checkbox
            checked={row.getIsSelected()}
            onCheckedChange={value => row.toggleSelected(!!value)}
            aria-label="Select row"
          />
        ),
        enableSorting: false,
        enableHiding: false,
      },
      ...alertsHistoryColumns,
    ],
    getCoreRowModel: getCoreRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    onSortingChange: setSorting,
    onColumnFiltersChange: setColumnFilters,
    onColumnVisibilityChange: setColumnVisibility,
    onRowSelectionChange: setRowSelection,
    globalFilterFn: (row, _columnId, filterValue) => {
      const system = row.original.expand?.system?.name || row.original.system || ""
      const name = row.getValue("name") || ""
      const search = String(filterValue).toLowerCase()
      return (
        system.toLowerCase().includes(search) ||
        String(name).toLowerCase().includes(search)
      )
    },
    state: {
      sorting,
      columnFilters,
      columnVisibility,
      rowSelection,
      globalFilter,
    },
    onGlobalFilterChange: setGlobalFilter,
  })

  // Bulk delete handler
  const handleBulkDelete = async () => {
    if (!window.confirm("Are you sure you want to delete the selected records?")) return
    const selectedIds = table.getSelectedRowModel().rows.map(row => row.original.id)
    try {
      await Promise.all(selectedIds.map(id => pb.collection("alerts_history").delete(id)))
      $alertsHistory.set(alertsHistory.filter(r => !selectedIds.includes(r.id)))
      toast.success("Deleted selected records.")
    } catch (e) {
      toast.error("Failed to delete some records.")
    }
  }

  // Export to CSV handler
  const handleExportCSV = () => {
    const selectedRows = table.getSelectedRowModel().rows
    if (!selectedRows.length) return
    const headers = ["system", "name", "value", "state", "created_date", "solved_date", "duration"]
    const csvRows = [headers.join(",")]
    for (const row of selectedRows) {
      const r = row.original
      csvRows.push([
        r.expand?.system?.name || r.system,
        r.name,
        r.value,
        r.state,
        r.created_date,
        r.solved_date,
        (() => {
          const created = r.created_date ? new Date(r.created_date) : null
          const solved = r.solved_date ? new Date(r.solved_date) : null
          if (!created || !solved) return ""
          const diffMs = solved.getTime() - created.getTime()
          if (diffMs < 0) return ""
          const totalSeconds = Math.floor(diffMs / 1000)
          const hours = Math.floor(totalSeconds / 3600)
          const minutes = Math.floor((totalSeconds % 3600) / 60)
          const seconds = totalSeconds % 60
          return [
            hours ? `${hours}h` : null,
            minutes ? `${minutes}m` : null,
            `${seconds}s`
          ].filter(Boolean).join(" ")
        })()
      ].map(v => `"${v ?? ""}"`).join(","))
    }
    const blob = new Blob([csvRows.join("\n")], { type: "text/csv" })
    const url = URL.createObjectURL(blob)
    const a = document.createElement("a")
    a.href = url
    a.download = "alerts_history.csv"
    a.click()
    URL.revokeObjectURL(url)
  }

  return (
    <div className="w-full">
      <div className="flex items-center py-4 gap-4">
        <Input
          placeholder="Filter system or name..."
          value={globalFilter}
          onChange={e => setGlobalFilter(e.target.value)}
          className="max-w-sm"
        />
        {table.getFilteredSelectedRowModel().rows.length > 0 && (
          <>
            <Button
              variant="destructive"
              onClick={handleBulkDelete}
              size="sm"
            >
              Delete Selected
            </Button>
            <Button
              variant="outline"
              onClick={handleExportCSV}
              size="sm"
            >
              Export Selected
            </Button>
          </>
        )}
      </div>
      <div className="rounded-md border overflow-x-auto">
        <Table>
          <TableHeader>
            {table.getHeaderGroups().map(headerGroup => (
              <TableRow key={headerGroup.id}>
                {headerGroup.headers.map(header => (
                  <TableHead key={header.id}>
                    {header.isPlaceholder
                      ? null
                      : flexRender(header.column.columnDef.header, header.getContext())}
                  </TableHead>
                ))}
              </TableRow>
            ))}
          </TableHeader>
          <TableBody>
            {table.getRowModel().rows.length ? (
              table.getRowModel().rows.map(row => (
                <TableRow
                  key={row.id}
                  data-state={row.getIsSelected() && "selected"}
                >
                  {row.getVisibleCells().map(cell => (
                    <TableCell key={cell.id}>
                      {flexRender(cell.column.columnDef.cell, cell.getContext())}
                    </TableCell>
                  ))}
                </TableRow>
              ))
            ) : (
              <TableRow>
                <TableCell colSpan={table.getAllColumns().length} className="h-24 text-center">
                  No results.
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </div>
      <div className="flex items-center justify-end space-x-2 py-4">
        <div className="text-muted-foreground flex-1 text-sm">
          {table.getFilteredSelectedRowModel().rows.length} of {table.getFilteredRowModel().rows.length} row(s) selected.
        </div>
        <div className="space-x-2">
          <Button
            variant="outline"
            size="sm"
            onClick={() => table.previousPage()}
            disabled={!table.getCanPreviousPage()}
          >
            Previous
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={() => table.nextPage()}
            disabled={!table.getCanNextPage()}
          >
            Next
          </Button>
        </div>
      </div>
    </div>
  )
} 