"use client"

import * as React from "react"
import {
  ColumnDef,
  flexRender,
  getCoreRowModel,
  getFilteredRowModel,
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
import { ProcessInfo } from "@/types"

interface TopProcessesProps {
  topCpuProcesses?: ProcessInfo[]
  topMemProcesses?: ProcessInfo[]
}

export const columns: ColumnDef<ProcessInfo>[] = [
  {
    accessorKey: "pid",
    header: () => "PID",
    cell: info => info.getValue(),
    size: 60,
  },
  {
    accessorKey: "name",
    header: ({ column }) => (
      <Button
        variant="ghost"
        onClick={() => column.toggleSorting(column.getIsSorted() === "asc")}
      >
        Name
        <ArrowUpDown className="ml-2 h-3 w-3" />
      </Button>
    ),
    cell: info => (
      <div className="font-medium truncate" title={info.row.original.name}>
        {info.getValue() as string}
      </div>
    ),
  },
  {
    accessorKey: "cmd",
    header: "CMD",
    cell: info => (
      <div className="text-xs text-muted-foreground truncate max-w-[150px]" title={info.row.original.cmd}>
        {info.getValue() as string}
      </div>
    ),
  },
  {
    accessorKey: "cpu",
    header: ({ column }) => (
      <Button
        variant="ghost"
        className="justify-end w-full"
        onClick={() => column.toggleSorting(column.getIsSorted() === "asc")}
      >
        CPU <ArrowUpDown className="ml-2 h-3 w-3" />
      </Button>
    ),
    cell: info => <span className="text-center block">{(info.getValue() as number).toFixed(1)}%</span>,
    size: 80,
  },
  {
    accessorKey: "mem",
    header: ({ column }) => (
      <Button
        variant="ghost"
        className="justify-end w-full"
        onClick={() => column.toggleSorting(column.getIsSorted() === "asc")}
      >
        MEM <ArrowUpDown className="ml-2 h-3 w-3" />
      </Button>
    ),
    cell: info => <span className="text-center block">{(info.getValue() as number).toFixed(1)}%</span>,
    size: 80,
  }
]

const PAGE_SIZE = 10

const TopProcesses: React.FC<TopProcessesProps> = ({ topCpuProcesses, topMemProcesses }) => {
  const processes = topCpuProcesses?.length ? topCpuProcesses : topMemProcesses
  const [sorting, setSorting] = React.useState<SortingState>([])
  const [columnVisibility, setColumnVisibility] = React.useState<VisibilityState>({})
  const [rowSelection, setRowSelection] = React.useState({})

  const table = useReactTable({
    data: processes ?? [],
    columns,
    onSortingChange: setSorting,
    getCoreRowModel: getCoreRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    onColumnVisibilityChange: setColumnVisibility,
    onRowSelectionChange: setRowSelection,
    state: {
      sorting,
      columnVisibility,
      rowSelection,
    },
  })

  if (!processes?.length) return null

  return (
    <div className="flex">
      <div className="rounded-md border max-w-2xl ml-auto">
        <Table>
          <TableHeader>
            {table.getHeaderGroups().map(headerGroup => (
              <TableRow key={headerGroup.id} className="h-10">
                {headerGroup.headers.map(header => (
                  <TableHead key={header.id} className="py-2 px-3 text-sm">
                    {header.isPlaceholder
                      ? null
                      : flexRender(header.column.columnDef.header, header.getContext())}
                  </TableHead>
                ))}
              </TableRow>
            ))}
          </TableHeader>
          <TableBody>
            {table.getRowModel().rows?.length ? (
              table.getRowModel().rows.map(row => (
                <TableRow
                  key={row.id}
                  data-state={row.getIsSelected() && "selected"}
                  className="h-10"
                >
                  {row.getVisibleCells().map(cell => (
                    <TableCell key={cell.id} className="py-2 px-3 text-sm">
                      {flexRender(cell.column.columnDef.cell, cell.getContext())}
                    </TableCell>
                  ))}
                </TableRow>
              ))
            ) : (
              <TableRow>
                <TableCell colSpan={columns.length} className="h-24 text-center text-sm">
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

export default React.memo(TopProcesses) 