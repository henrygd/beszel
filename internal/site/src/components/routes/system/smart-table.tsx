import * as React from "react"
import { t } from "@lingui/core/macro"
import {
   ColumnDef,
   ColumnFiltersState,
   flexRender,
   getCoreRowModel,
   getFilteredRowModel,
   getSortedRowModel,
   SortingState,
   useReactTable,
} from "@tanstack/react-table"
import { Activity, Box, Clock, HardDrive, HashIcon, CpuIcon, BinaryIcon, RotateCwIcon, LoaderCircleIcon, CheckCircle2Icon, XCircleIcon, ArrowLeftRightIcon } from "lucide-react"
import { Card, CardHeader, CardTitle, CardDescription } from "@/components/ui/card"
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet"
import { Input } from "@/components/ui/input"
import {
   Table,
   TableBody,
   TableCell,
   TableHead,
   TableHeader,
   TableRow,
} from "@/components/ui/table"
import { Badge } from "@/components/ui/badge"
import { pb } from "@/lib/api"
import { SmartData, SmartAttribute } from "@/types"
import { formatBytes, toFixedFloat, formatTemperature } from "@/lib/utils"
import { Trans } from "@lingui/react/macro"
import { ThermometerIcon } from "@/components/ui/icons"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Separator } from "@/components/ui/separator"

// Column definition for S.M.A.R.T. attributes table
export const smartColumns: ColumnDef<SmartAttribute>[] = [
   {
      accessorKey: "id",
      header: "ID",
      cell: ({ getValue }) => {
         const id = getValue() as number | undefined
         return <div className="font-mono text-sm">{id?.toString() || ""}</div>
      },
   },
   {
      accessorFn: (row) => row.n,
      header: "Name",
   },
   {
      accessorFn: (row) => row.rs || row.rv?.toString(),
      header: "Value",
   },
   {
      accessorKey: "v",
      header: "Normalized",
   },
   {
      accessorKey: "w",
      header: "Worst",
   },
   {
      accessorKey: "t",
      header: "Threshold",
   },
   {
      // accessorFn: (row) => row.wf,
      accessorKey: "wf",
      header: "Failing",
   },
]



export type DiskInfo = {
   device: string
   model: string
   serialNumber: string
   firmwareVersion: string
   capacity: string
   status: string
   temperature: number
   deviceType: string
   powerOnHours?: number
   powerCycles?: number
}

// Function to format capacity display
function formatCapacity(bytes: number): string {
   const { value, unit } = formatBytes(bytes)
   return `${toFixedFloat(value, value >= 10 ? 1 : 2)} ${unit}`
}

// Function to convert SmartData to DiskInfo
function convertSmartDataToDiskInfo(smartDataRecord: Record<string, SmartData>): DiskInfo[] {
   return Object.entries(smartDataRecord).map(([key, smartData]) => ({
      device: smartData.dn || key,
      model: smartData.mn || "Unknown",
      serialNumber: smartData.sn || "Unknown",
      firmwareVersion: smartData.fv || "Unknown",
      capacity: smartData.c ? formatCapacity(smartData.c) : "Unknown",
      status: smartData.s || "Unknown",
      temperature: smartData.t || 0,
      deviceType: smartData.dt || "Unknown",
      // These fields need to be extracted from SmartAttribute if available
      powerOnHours: smartData.a?.find(attr => attr.n.toLowerCase().includes("poweronhours") || attr.n.toLowerCase().includes("power_on_hours"))?.rv,
      powerCycles: smartData.a?.find(attr => attr.n.toLowerCase().includes("power") && attr.n.toLowerCase().includes("cycle"))?.rv,
   }))
}


export const columns: ColumnDef<DiskInfo>[] = [
   {
      accessorKey: "device",
      header: () => (
         <div className="flex items-center gap-1.5">
            <HardDrive className="size-4" />
            <Trans>Device</Trans>
         </div>
      ),
      cell: ({ row }) => (
         <div className="font-medium">{row.getValue("device")}</div>
      ),
   },
   {
      accessorKey: "model",
      header: () => (
         <div className="flex items-center gap-1.5">
            <Box className="size-4" />
            <Trans>Model</Trans>
         </div>
      ),
      cell: ({ row }) => (
         <div className="max-w-50 truncate" title={row.getValue("model")}>
            {row.getValue("model")}
         </div>
      ),
   },
   {
      accessorKey: "capacity",
      header: () => (
         <div className="flex items-center gap-1.5">
            <BinaryIcon className="size-4" />
            <Trans>Capacity</Trans>
         </div>
      ),
   },
   {
      accessorKey: "temperature",
      header: () => (
         <div className="flex items-center gap-2">
            <ThermometerIcon className="size-4" />
            <Trans>Temp</Trans>
         </div>
      ),
      cell: ({ getValue }) => {
         const { value, unit } = formatTemperature(getValue() as number)
         return `${value} ${unit}`
      },
   },
   {
      accessorKey: "status",
      header: () => (
         <div className="flex items-center gap-2">
            <Activity className="size-4" />
            <Trans>Status</Trans>
         </div>
      ),
      cell: ({ getValue }) => {
         const status = getValue() as string
         return (
            <Badge
               variant={status === "PASSED" ? "success" : status === "FAILED" ? "danger" : "warning"}
            >
               {status}
            </Badge>
         )
      },
   },
   {
      accessorKey: "deviceType",
      header: () => (
         <div className="flex items-center gap-1.5">
            <ArrowLeftRightIcon className="size-4" />
            <Trans>Type</Trans>
         </div>
      ),
      cell: ({ getValue }) => (
         <Badge variant="outline" className="uppercase">
            {getValue() as string}
         </Badge>
      ),
   },
   {
      accessorKey: "powerOnHours",
      header: () => (
         <div className="flex items-center gap-1.5">
            <Clock className="size-4" />
            <Trans comment="Power On Time">Power On</Trans>
         </div>
      ),
      cell: ({ row }) => {
         const hours = row.getValue("powerOnHours") as number | undefined
         if (!hours && hours !== 0) {
            return (
               <div className="text-sm text-muted-foreground">
                  N/A
               </div>
            )
         }
         const days = Math.floor(hours / 24)
         return (
            <div className="text-sm">
               <div>{hours.toLocaleString()} hours</div>
               <div className="text-muted-foreground text-xs">{days} days</div>
            </div>
         )
      },
   },
   {
      accessorKey: "powerCycles",
      header: () => (
         <div className="flex items-center gap-1.5">
            <RotateCwIcon className="size-4" />
            <Trans comment="Power Cycles">Cycles</Trans>
         </div>
      ),
      cell: ({ getValue }) => {
         const cycles = getValue() as number | undefined
         if (!cycles && cycles !== 0) {
            return (
               <div className="text-muted-foreground">
                  N/A
               </div>
            )
         }
         return cycles
      },
   },
   {
      accessorKey: "serialNumber",
      header: () => (
         <div className="flex items-center gap-1.5">
            <HashIcon className="size-4" />
            <Trans>Serial Number</Trans>
         </div>
      ),
   },
   {
      accessorKey: "firmwareVersion",
      header: () => (
         <div className="flex items-center gap-1.5">
            <CpuIcon className="size-4" />
            <Trans>Firmware</Trans>
         </div>
      ),
   },
]

export default function DisksTable({ systemId }: { systemId: string }) {
   const [sorting, setSorting] = React.useState<SortingState>([{ id: "device", desc: false }])
   const [columnFilters, setColumnFilters] = React.useState<ColumnFiltersState>([])
   const [rowSelection, setRowSelection] = React.useState({})
   const [smartData, setSmartData] = React.useState<Record<string, SmartData> | undefined>(undefined)
   const [activeDisk, setActiveDisk] = React.useState<DiskInfo | null>(null)
   const [sheetOpen, setSheetOpen] = React.useState(false)

   const openSheet = (disk: DiskInfo) => {
      setActiveDisk(disk)
      setSheetOpen(true)
   }

   // Fetch smart data when component mounts or systemId changes
   React.useEffect(() => {
      if (systemId) {
         pb.send<Record<string, SmartData>>("/api/beszel/smart", { query: { system: systemId } })
            .then((data) => {
               setSmartData(data)
            })
            .catch(() => setSmartData({}))
      }
   }, [systemId])

   // Convert SmartData to DiskInfo, if no data use empty array
   const diskData = React.useMemo(() => {
      return smartData ? convertSmartDataToDiskInfo(smartData) : []
   }, [smartData])


   const table = useReactTable({
      data: diskData,
      columns: columns,
      onSortingChange: setSorting,
      onColumnFiltersChange: setColumnFilters,
      getCoreRowModel: getCoreRowModel(),
      getSortedRowModel: getSortedRowModel(),
      getFilteredRowModel: getFilteredRowModel(),
      onRowSelectionChange: setRowSelection,
      state: {
         sorting,
         columnFilters,
         rowSelection,
      },
   })

   return (
      <div>
         <Card className="p-6 @container w-full">
            <CardHeader className="p-0 mb-4">
               <div className="grid md:flex gap-5 w-full items-end">
                  <div className="px-2 sm:px-1">
                     <CardTitle className="mb-2">
                        S.M.A.R.T.
                     </CardTitle>
                     <CardDescription className="flex">
                        <Trans>Click on a device to view more information.</Trans>
                     </CardDescription>
                  </div>
                  <Input
                     placeholder={t`Filter...`}
                     value={(table.getColumn("device")?.getFilterValue() as string) ?? ""}
                     onChange={(event) =>
                        table.getColumn("device")?.setFilterValue(event.target.value)
                     }
                     className="ms-auto px-4 w-full max-w-full md:w-64"
                  />
               </div>
            </CardHeader>
            <div className="rounded-md border text-nowrap">
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
                              className="cursor-pointer"
                              onClick={() => openSheet(row.original)}
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
                              {smartData ? t`No results.` : <LoaderCircleIcon className="animate-spin size-10 opacity-60 mx-auto" />}

                           </TableCell>
                        </TableRow>
                     )}
                  </TableBody>
               </Table>
            </div>
         </Card>
         <DiskSheet disk={activeDisk} smartData={activeDisk && smartData ? Object.values(smartData).find(sd => sd.dn === activeDisk.device || sd.mn === activeDisk.model) : undefined} open={sheetOpen} onOpenChange={setSheetOpen} />
      </div>
   )
}

function DiskSheet({ disk, smartData, open, onOpenChange }: { disk: DiskInfo | null; smartData?: SmartData; open: boolean; onOpenChange: (open: boolean) => void }) {
   if (!disk) return null

   const [sorting, setSorting] = React.useState<SortingState>([{ id: "id", desc: false }])

   const smartAttributes = smartData?.a || []

   // Find all attributes where when failed is not empty
   const failedAttributes = smartAttributes.filter(attr => attr.wf && attr.wf.trim() !== '')

   // Filter columns to only show those that have values in at least one row
   const visibleColumns = React.useMemo(() => {
      return smartColumns.filter(column => {
         const accessorKey = (column as any).accessorKey as keyof SmartAttribute
         if (!accessorKey) {
            return true
         }
         // Check if any row has a non-empty value for this column
         return smartAttributes.some(attr => {
            return attr[accessorKey] !== undefined
         })
      })
   }, [smartAttributes])

   const table = useReactTable({
      data: smartAttributes,
      columns: visibleColumns,
      getCoreRowModel: getCoreRowModel(),
      getSortedRowModel: getSortedRowModel(),
      onSortingChange: setSorting,
      state: {
         sorting,
      }
   })

   return (
      <Sheet open={open} onOpenChange={onOpenChange}>
         <SheetContent className="w-full sm:max-w-220 gap-0">
            <SheetHeader className="mb-0 border-b">
               <SheetTitle><Trans>S.M.A.R.T. Details</Trans> - {disk.device}</SheetTitle>
               <SheetDescription className="flex flex-wrap items-center gap-x-2 gap-y-1">
                  {disk.model} <Separator orientation="vertical" className="h-2.5 bg-muted-foreground opacity-70" />
                  {disk.serialNumber}
               </SheetDescription>
            </SheetHeader>
            <div className="flex-1 overflow-auto p-4 flex flex-col gap-4">
               <Alert className="pb-3">
                  {smartData?.s === "PASSED" ? (
                     <CheckCircle2Icon className="size-4" />
                  ) : (
                     <XCircleIcon className="size-4" />
                  )}
                  <AlertTitle><Trans>S.M.A.R.T. Self-Test</Trans>: {smartData?.s}</AlertTitle>
                  {failedAttributes.length > 0 && (
                     <AlertDescription>
                        <Trans>Failed Attributes:</Trans> {failedAttributes.map(attr => attr.n).join(", ")}
                     </AlertDescription>
                  )}
               </Alert>
               {smartAttributes.length > 0 ? (
                  <div className="rounded-md border overflow-auto">
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
                           {table.getRowModel().rows.map((row) => {
                              // Check if the attribute is failed
                              const isFailedAttribute = row.original.wf && row.original.wf.trim() !== '';

                              return (
                                 <TableRow
                                    key={row.id}
                                    className={isFailedAttribute ? "text-red-600 dark:text-red-400" : ""}
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
                              );
                           })}
                        </TableBody>
                     </Table>
                  </div>
               ) : (
                  <div className="text-center py-8 text-muted-foreground">
                     <Trans>No S.M.A.R.T. attributes available for this device.</Trans>
                  </div>
               )}
            </div>
         </SheetContent>
      </Sheet>
   )
} 