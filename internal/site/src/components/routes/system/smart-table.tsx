import * as React from "react"
import { t } from "@lingui/core/macro"
import {
   ColumnDef,
   ColumnFiltersState,
   Column,
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
import { Button } from "@/components/ui/button"
import { pb } from "@/lib/api"
import { SmartData, SmartAttribute } from "@/types"
import { formatBytes, toFixedFloat, formatTemperature, cn, secondsToString } from "@/lib/utils"
import { Trans } from "@lingui/react/macro"
import { ThermometerIcon } from "@/components/ui/icons"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Separator } from "@/components/ui/separator"

// Column definition for S.M.A.R.T. attributes table
export const smartColumns: ColumnDef<SmartAttribute>[] = [
   {
      accessorKey: "id",
      header: "ID",
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
   const unknown = "Unknown"
   return Object.entries(smartDataRecord).map(([key, smartData]) => ({
      device: smartData.dn || key,
      model: smartData.mn || unknown,
      serialNumber: smartData.sn || unknown,
      firmwareVersion: smartData.fv || unknown,
      capacity: smartData.c ? formatCapacity(smartData.c) : unknown,
      status: smartData.s || unknown,
      temperature: smartData.t || 0,
      deviceType: smartData.dt || unknown,
      // These fields need to be extracted from SmartAttribute if available
      powerOnHours: smartData.a?.find(attr => {
         const name = attr.n.toLowerCase();
         return name.includes("poweronhours") || name.includes("power_on_hours");
      })?.rv,
      powerCycles: smartData.a?.find(attr => {
         const name = attr.n.toLowerCase();
         return (name.includes("power") && name.includes("cycle")) || name.includes("startstopcycles");
      })?.rv,
   }))
}


export const columns: ColumnDef<DiskInfo>[] = [
   {
      accessorKey: "device",
      sortingFn: (a, b) => a.original.device.localeCompare(b.original.device),
      header: ({ column }) => <HeaderButton column={column} name={t`Device`} Icon={HardDrive} />,
      cell: ({ row }) => (
         <div className="font-medium ms-1.5">{row.getValue("device")}</div>
      ),
   },
   {
      accessorKey: "model",
      sortingFn: (a, b) => a.original.model.localeCompare(b.original.model),
      header: ({ column }) => <HeaderButton column={column} name={t`Model`} Icon={Box} />,
      cell: ({ row }) => (
         <div className="max-w-50 truncate ms-1.5" title={row.getValue("model")}>
            {row.getValue("model")}
         </div>
      ),
   },
   {
      accessorKey: "capacity",
      header: ({ column }) => <HeaderButton column={column} name={t`Capacity`} Icon={BinaryIcon} />,
      cell: ({ getValue }) => (
         <span className="ms-1.5">{getValue() as string}</span>
      ),
   },
   {
      accessorKey: "temperature",
      invertSorting: true,
      header: ({ column }) => <HeaderButton column={column} name={t`Temp`} Icon={ThermometerIcon} />,
      cell: ({ getValue }) => {
         const { value, unit } = formatTemperature(getValue() as number)
         return <span className="ms-1.5">{`${value} ${unit}`}</span>
      },
   },
   {
      accessorKey: "status",
      header: ({ column }) => <HeaderButton column={column} name={t`Status`} Icon={Activity} />,
      cell: ({ getValue }) => {
         const status = getValue() as string
         return (
            <div className="ms-1.5">
               <Badge
                  variant={status === "PASSED" ? "success" : status === "FAILED" ? "danger" : "warning"}
               >
                  {status}
               </Badge>
            </div>
         )
      },
   },
   {
      accessorKey: "deviceType",
      sortingFn: (a, b) => a.original.deviceType.localeCompare(b.original.deviceType),
      header: ({ column }) => <HeaderButton column={column} name={t`Type`} Icon={ArrowLeftRightIcon} />,
      cell: ({ getValue }) => (
         <div className="ms-1.5">
            <Badge variant="outline" className="uppercase">
               {getValue() as string}
            </Badge>
         </div>
      ),
   },
   {
      accessorKey: "powerOnHours",
      invertSorting: true,
      header: ({ column }) => <HeaderButton column={column} name={t({ message: "Power On", comment: "Power On Time" })} Icon={Clock} />,
      cell: ({ getValue }) => {
         const hours = (getValue() ?? 0) as number
         if (!hours && hours !== 0) {
            return (
               <div className="text-sm text-muted-foreground ms-1.5">
                  N/A
               </div>
            )
         }
         const seconds = hours * 3600
         return (
            <div className="text-sm ms-1.5">
               <div>{secondsToString(seconds, "hour")}</div>
               <div className="text-muted-foreground text-xs">{secondsToString(seconds, "day")}</div>
            </div>
         )
      },
   },
   {
      accessorKey: "powerCycles",
      invertSorting: true,
      header: ({ column }) => <HeaderButton column={column} name={t({ message: "Cycles", comment: "Power Cycles" })} Icon={RotateCwIcon} />,
      cell: ({ getValue }) => {
         const cycles = getValue() as number | undefined
         if (!cycles && cycles !== 0) {
            return (
               <div className="text-muted-foreground ms-1.5">
                  N/A
               </div>
            )
         }
         return <span className="ms-1.5">{cycles}</span>
      },
   },
   {
      accessorKey: "serialNumber",
      sortingFn: (a, b) => a.original.serialNumber.localeCompare(b.original.serialNumber),
      header: ({ column }) => <HeaderButton column={column} name={t`Serial Number`} Icon={HashIcon} />,
      cell: ({ getValue }) => (
         <span className="ms-1.5">{getValue() as string}</span>
      ),
   },
   {
      accessorKey: "firmwareVersion",
      sortingFn: (a, b) => a.original.firmwareVersion.localeCompare(b.original.firmwareVersion),
      header: ({ column }) => <HeaderButton column={column} name={t`Firmware`} Icon={CpuIcon} />,
      cell: ({ getValue }) => (
         <span className="ms-1.5">{getValue() as string}</span>
      ),
   },
]

function HeaderButton({ column, name, Icon }: { column: Column<DiskInfo>; name: string; Icon: React.ElementType }) {
   const isSorted = column.getIsSorted()
   return (
      <Button
         className={cn("h-9 px-3 flex items-center gap-2 duration-50", isSorted && "bg-accent/70 light:bg-accent text-accent-foreground/90")}
         variant="ghost"
         onClick={() => column.toggleSorting(column.getIsSorted() === "asc")}
      >
         {Icon && <Icon className="size-4" />}
         {name}
      </Button>
   )
}

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

   if (!diskData.length && !columnFilters.length) {
      return null
   }

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
                                 <TableHead key={header.id} className="px-2">
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
                                 <TableCell key={cell.id} className="md:ps-5">
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
         <DiskSheet disk={activeDisk} smartData={smartData?.[activeDisk?.serialNumber ?? ""]} open={sheetOpen} onOpenChange={setSheetOpen} />
      </div>
   )
}

function DiskSheet({ disk, smartData, open, onOpenChange }: { disk: DiskInfo | null; smartData?: SmartData; open: boolean; onOpenChange: (open: boolean) => void }) {
   if (!disk) return null

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