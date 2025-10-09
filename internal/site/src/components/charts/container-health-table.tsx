"use client"

import * as React from "react"
import {
  ColumnDef,
  flexRender,
  getCoreRowModel,
  getPaginationRowModel,
  getSortedRowModel,
  SortingState,
  useReactTable,
  VisibilityState,
} from "@tanstack/react-table"
import { ArrowUpDown } from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { Badge } from "@/components/ui/badge"
import { cn } from "@/lib/utils"

export type DockerHealthRow = {
  id: string
  name: string
  stack: string
  health: string
  healthValue: number
  status: string
  uptime: string
  color: string
}

type Props = {
  data: DockerHealthRow[]
}

export function ContainerHealthTable({ data }: Props) {
  const [sorting, setSorting] = React.useState<SortingState>([])
  const [columnVisibility, setColumnVisibility] = React.useState<VisibilityState>({})
  const [rowSelection, setRowSelection] = React.useState({})

  const columns = React.useMemo<ColumnDef<DockerHealthRow>[]>(() => [
    {
      accessorKey: "id",
      header: ({ column }) => (
        <Button
          variant="ghost"
          onClick={() => column.toggleSorting(column.getIsSorted() === "asc")}
        >
          ID
          <ArrowUpDown className="ml-2 h-4 w-4" />
        </Button>
      ),
      cell: ({ row }) => (
        <span className="font-mono text-xs text-muted-foreground" title={row.original.id}>{row.original.id}</span>
      ),
    },
    {
      accessorKey: "name",
      header: ({ column }) => (
        <Button
          variant="ghost"
          onClick={() => column.toggleSorting(column.getIsSorted() === "asc")}
        >
          Container
          <ArrowUpDown className="ml-2 h-4 w-4" />
        </Button>
      ),
      cell: ({ row }) => (
        <div className="flex items-center gap-1.5">
          <div className="w-2.5 h-2.5 rounded-full flex-shrink-0" style={{ backgroundColor: row.original.color }} />
          <span className="text-xs truncate">{row.original.name}</span>
        </div>
      ),
    },
    {
      accessorKey: "stack",
      header: ({ column }) => (
        <Button
          variant="ghost"
          onClick={() => column.toggleSorting(column.getIsSorted() === "asc")}
        >
          Stack
          <ArrowUpDown className="ml-2 h-4 w-4" />
        </Button>
      ),
      cell: ({ row }) => (
        <span className="text-xs text-muted-foreground truncate" title={row.original.stack}>{row.original.stack}</span>
      ),
    },
    {
      accessorKey: "health",
      header: ({ column }) => (
        <Button
          variant="ghost"
          onClick={() => column.toggleSorting(column.getIsSorted() === "asc")}
        >
          Health
          <ArrowUpDown className="ml-2 h-4 w-4" />
        </Button>
      ),
      cell: ({ row }) => (
        <Badge className={cn(
          "px-1.5 py-0.5 text-xs font-medium whitespace-nowrap border-0",
          {
            "bg-green-100 text-green-800 dark:bg-green-900/20 dark:text-green-400": row.original.healthValue === 3,
            "bg-yellow-100 text-yellow-800 dark:bg-yellow-900/20 dark:text-yellow-400": row.original.healthValue === 2,
            "bg-red-100 text-red-800 dark:bg-red-900/20 dark:text-red-400": row.original.healthValue === 1,
            "bg-gray-100 text-gray-800 dark:bg-gray-900/20 dark:text-gray-400": row.original.healthValue === 0,
          }
        )}>{row.original.health}</Badge>
      ),
    },
    {
      accessorKey: "status",
      header: ({ column }) => (
        <Button
          variant="ghost"
          onClick={() => column.toggleSorting(column.getIsSorted() === "asc")}
        >
          Status
          <ArrowUpDown className="ml-2 h-4 w-4" />
        </Button>
      ),
      cell: ({ row }) => (
        <Badge className={cn(
          "px-1.5 py-0.5 text-xs font-medium whitespace-nowrap border-0",
          {
            "bg-green-100 text-green-800 dark:bg-green-900/20 dark:text-green-400": row.original.status?.toLowerCase() === "running",
            "bg-yellow-100 text-yellow-800 dark:bg-yellow-900/20 dark:text-yellow-400": row.original.status?.toLowerCase() === "paused" || row.original.status?.toLowerCase() === "restarting",
            "bg-red-100 text-red-800 dark:bg-red-900/20 dark:text-red-400":
              row.original.status?.toLowerCase().includes("exited") ||
              row.original.status?.toLowerCase().includes("dead") ||
              row.original.status?.toLowerCase().includes("removing"),
            "bg-gray-100 text-gray-800 dark:bg-gray-900/20 dark:text-gray-400": !row.original.status || row.original.status?.toLowerCase() === "created",
          }
        )} title={row.original.status}>{row.original.status}</Badge>
      ),
    },
    {
      accessorKey: "uptime",
      header: ({ column }) => (
        <Button
          variant="ghost"
          onClick={() => column.toggleSorting(column.getIsSorted() === "asc")}
        >
          Uptime
          <ArrowUpDown className="ml-2 h-4 w-4" />
        </Button>
      ),
      cell: ({ row }) => (
        <span className="text-xs whitespace-nowrap">{row.original.uptime}</span>
      ),
    },
  ], [])

  const table = useReactTable({
    data,
    columns,
    onSortingChange: setSorting,
    getCoreRowModel: getCoreRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
    getSortedRowModel: getSortedRowModel(),
    onColumnVisibilityChange: setColumnVisibility,
    onRowSelectionChange: setRowSelection,
    state: {
      sorting,
      columnVisibility,
      rowSelection,
    },
  })

  React.useEffect(() => {
    table.setPageSize(5)
  }, [table])

  return (
    <div className="w-full">
      <div className="rounded-md border">
        <Table>
          <TableHeader>
            {table.getHeaderGroups().map((headerGroup) => (
              <TableRow key={headerGroup.id}>
                {headerGroup.headers.map((header) => {
                  return (
                    <TableHead key={header.id}>
                      {header.isPlaceholder
                        ? null
                        : flexRender(
                            header.column.columnDef.header,
                            header.getContext()
                          )}
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
                  colSpan={columns.length}
                  className="h-24 text-center"
                >
                  No results.
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </div>
      <div className="flex items-center justify-end space-x-2 py-1">
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