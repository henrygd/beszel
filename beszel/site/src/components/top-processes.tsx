import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "./ui/table"
import { ProcessInfo } from "@/types"
import { memo } from "react"
import { cn } from "@/lib/utils"

interface TopProcessesProps {
	topCpuProcesses?: ProcessInfo[]
	topMemProcesses?: ProcessInfo[]
}

const TopProcesses = memo(({ topCpuProcesses, topMemProcesses }: TopProcessesProps) => {
	// Show either CPU or memory processes, not both
	const processes = topCpuProcesses?.length ? topCpuProcesses : topMemProcesses

	if (!processes?.length) {
		return null
	}

	return (
		<div>
			<div className={cn("h-full w-full absolute aspect-auto bg-card opacity-0 transition-opacity", {"opacity-100": true,})}>
				<Table>
					<TableHeader>
						<TableRow>
							<TableHead className="w-12">PID</TableHead>
							<TableHead>Name</TableHead>
							<TableHead>CMD</TableHead>
							<TableHead className="w-16 text-right">CPU</TableHead>
							<TableHead className="w-16 text-right">MEM</TableHead>
						</TableRow>
					</TableHeader>
					<TableBody>
						{processes.map((process, index) => (
							<TableRow key={`${process.pid}-${index}`}>
								<TableCell className="text-sm">{process.pid}</TableCell>
								<TableCell>
									<div className="font-medium truncate" title={process.name}>
										{process.name}
									</div>
								</TableCell>
								<TableCell>
									<div className="text-xs text-muted-foreground truncate max-w-[200px]" title={process.cmd}>
										{process.cmd}
									</div>
								</TableCell>
								<TableCell className="text-right">
									{process.cpu.toFixed(1)}%
								</TableCell>
								<TableCell className="text-right">
									{process.mem.toFixed(1)}%
								</TableCell>
							</TableRow>
						))}
					</TableBody>
				</Table>
			</div>
		</div>
	)
})

TopProcesses.displayName = "TopProcesses"

export default TopProcesses 