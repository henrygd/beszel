import { ColumnDef } from "@tanstack/react-table"
import { AlertsHistoryRecord } from "@/types"
import { Button } from "@/components/ui/button"
import { ArrowUpDown } from "lucide-react"
import { Badge } from "@/components/ui/badge"

export const alertsHistoryColumns: ColumnDef<AlertsHistoryRecord>[] = [
  {
    accessorKey: "system",
    header: ({ column }) => (
      <Button
        variant="ghost"
        onClick={() => column.toggleSorting(column.getIsSorted() === "asc")}
      >
        System <ArrowUpDown className="ml-2 h-4 w-4" />
      </Button>
    ),
    cell: ({ row }) => <span className="text-center block">{row.original.expand?.system?.name || row.original.system}</span>,
    enableSorting: true,
    filterFn: (row, _, filterValue) => {
      const display = row.original.expand?.system?.name || row.original.system || ""
      return display.toLowerCase().includes(filterValue.toLowerCase())
    },
  },
  {
    accessorKey: "name",
    header: ({ column }) => (
      <Button
        variant="ghost"
        onClick={() => column.toggleSorting(column.getIsSorted() === "asc")}
      >
        Name <ArrowUpDown className="ml-2 h-4 w-4" />
      </Button>
    ),
    cell: ({ row }) => <span className="text-center block">{row.getValue("name")}</span>,
    enableSorting: true,
    filterFn: (row, _, filterValue) => {
      const value = row.getValue("name") || ""
      return String(value).toLowerCase().includes(filterValue.toLowerCase())
    },
  },
  {
    accessorKey: "value",
    header: ({ column }) => (
      <Button
        variant="ghost"
        onClick={() => column.toggleSorting(column.getIsSorted() === "asc")}
        className="text-right w-full justify-end"
      >
        Value <ArrowUpDown className="ml-2 h-4 w-4" />
      </Button>
    ),
    cell: ({ row }) => <span className="text-center block">{Math.round(Number(row.getValue("value")))}</span>,
    enableSorting: true,
  },
  {
    accessorKey: "state",
    header: ({ column }) => (
      <Button
        variant="ghost"
        onClick={() => column.toggleSorting(column.getIsSorted() === "asc")}
        className="text-center w-full justify-start"
      >
        State <ArrowUpDown className="ml-2 h-4 w-4" />
      </Button>
    ),
    cell: ({ row }) => {
      const state = row.getValue("state") as string
      let color = ""
      if (state === "solved") color = "bg-green-100 text-green-800 border-green-200"
      else if (state === "active") color = "bg-yellow-100 text-yellow-800 border-yellow-200"
      return (
        <span className="text-center block">
          <Badge className={`capitalize ${color}`}>{state}</Badge>
        </span>
      )
    },
    enableSorting: true,
  },
  {
    accessorKey: "create_date",
    header: ({ column }) => (
      <Button
        variant="ghost"
        onClick={() => column.toggleSorting(column.getIsSorted() === "asc")}
      >
        Created <ArrowUpDown className="ml-2 h-4 w-4" />
      </Button>
    ),
    cell: ({ row }) => (
      <span className="text-center block">
        {row.original.created_date ? new Date(row.original.created_date).toLocaleString() : ""}
      </span>
    ),
    enableSorting: true,
  },
  {
    accessorKey: "solved_date",
    header: ({ column }) => (
      <Button
        variant="ghost"
        onClick={() => column.toggleSorting(column.getIsSorted() === "asc")}
      >
        Solved <ArrowUpDown className="ml-2 h-4 w-4" />
      </Button>
    ),
    cell: ({ row }) => (
      <span className="text-center block">
        {row.original.solved_date ? new Date(row.original.solved_date).toLocaleString() : ""}
      </span>
    ),
    enableSorting: true,
  },
  {
    accessorKey: "duration",
    header: ({ column }) => (
      <Button
        variant="ghost"
        onClick={() => column.toggleSorting(column.getIsSorted() === "asc")}
      >
        Duration <ArrowUpDown className="ml-2 h-4 w-4" />
      </Button>
    ),
    cell: ({ row }) => {
      const created = row.original.created_date ? new Date(row.original.created_date) : null
      const solved = row.original.solved_date ? new Date(row.original.solved_date) : null
      if (!created || !solved) return <span className="text-center block"></span>
      const diffMs = solved.getTime() - created.getTime()
      if (diffMs < 0) return <span className="text-center block"></span>
      const totalSeconds = Math.floor(diffMs / 1000)
      const hours = Math.floor(totalSeconds / 3600)
      const minutes = Math.floor((totalSeconds % 3600) / 60)
      const seconds = totalSeconds % 60
      return (
        <span className="text-center block">
          {[
            hours ? `${hours}h` : null,
            minutes ? `${minutes}m` : null,
            `${seconds}s`
          ].filter(Boolean).join(" ")}
        </span>
      )
    },
    enableSorting: true,
  },
] 