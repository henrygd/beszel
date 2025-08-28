import * as React from "react"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { XIcon, ChevronDownIcon, PlusIcon } from "lucide-react"
import { type InputProps } from "./input"
import { cn } from "@/lib/utils"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger, DropdownMenuSeparator, DropdownMenuLabel } from "./dropdown-menu"
import { Command, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList } from "./command"
import { useStore } from "@nanostores/react"
import { $systems } from "@/lib/stores"

type InputTagsProps = Omit<InputProps, "value" | "onChange"> & {
	value: string[]
	onChange: React.Dispatch<React.SetStateAction<string[]>>
}

const InputTags = React.forwardRef<HTMLInputElement, InputTagsProps>(
	({ className, value, onChange, ...props }, ref) => {
		const [pendingDataPoint, setPendingDataPoint] = React.useState("")
		const [dropdownOpen, setDropdownOpen] = React.useState(false)
		const systems = useStore($systems)

		// Optimized existing tags computation
		const existingTags = React.useMemo(() => {
			const tagSet = new Set<string>()
			for (const system of systems) {
				if (system.tags) {
					for (const tag of system.tags) {
						tagSet.add(tag)
					}
				}
			}
			return Array.from(tagSet).sort()
		}, [systems])

		// Optimized filtering of available tags
		const availableTags = React.useMemo(() => {
			const selectedSet = new Set(value)
			return existingTags.filter(tag => !selectedSet.has(tag))
		}, [existingTags, value])

		// Optimized comma handling with useCallback
		const handleCommaInput = React.useCallback(() => {
			if (pendingDataPoint.includes(",")) {
				const newTags = pendingDataPoint.split(",").map(chunk => chunk.trim()).filter(Boolean)
				const combinedTags = new Set([...value, ...newTags])
				onChange(Array.from(combinedTags))
				setPendingDataPoint("")
			}
		}, [pendingDataPoint, value, onChange])

		React.useEffect(() => {
			handleCommaInput()
		}, [handleCommaInput])

		// Optimized tag operations with useCallback
		const addPendingDataPoint = React.useCallback(() => {
			const trimmed = pendingDataPoint.trim()
			if (trimmed && !value.includes(trimmed)) {
				onChange([...value, trimmed])
				setPendingDataPoint("")
			}
		}, [pendingDataPoint, value, onChange])

		const addTag = React.useCallback((tag: string) => {
			if (!value.includes(tag)) {
				onChange([...value, tag])
			}
		}, [value, onChange])

		return (
			<div className="flex items-start gap-2">
				<div
					className={cn(
						"bg-background min-h-10 flex flex-wrap items-center max-w-[30rem] gap-2 rounded-md border px-3 py-2 text-sm placeholder:text-muted-foreground has-[:focus-visible]:outline-none ring-offset-background has-[:focus-visible]:ring-2 has-[:focus-visible]:ring-ring has-[:focus-visible]:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50 overflow-y-auto",
						// Height growth and border color feedback
						"transition-all duration-200 ease-in-out",
						value.length > 4 && "min-h-16 border-blue-200 dark:border-blue-800",
						value.length > 8 && "min-h-24 border-blue-300 dark:border-blue-700",
						value.length > 12 && "min-h-32 border-blue-400 dark:border-blue-600",
						value.length > 16 && "min-h-40 border-blue-500 dark:border-blue-500",
						className
					)}
					style={{maxHeight: '10rem'}}
				>
					{value.map((item) => (
						<Badge key={item} className="flex-shrink-0">
							{item}
							<Button
								variant="ghost"
								size="icon"
								className="ms-2 h-3 w-3"
								onClick={() => {
									onChange(value.filter((i) => i !== item))
								}}
							>
								<XIcon className="w-3" />
							</Button>
						</Badge>
					))}
					<input
						className="flex-1 min-w-0 min-w-[100px] outline-none bg-background placeholder:text-muted-foreground max-w-full"
						value={pendingDataPoint}
						onChange={(e) => setPendingDataPoint(e.target.value)}
						onKeyDown={(e) => {
							if (e.key === "Enter" || e.key === "," || e.key === "Tab") {
								e.preventDefault()
								addPendingDataPoint()
							} else if (e.key === "Backspace" && pendingDataPoint.length === 0 && value.length > 0) {
								e.preventDefault()
								onChange(value.slice(0, -1))
							}
						}}
						{...props}
						ref={ref}
					/>
				</div>
				<DropdownMenu open={dropdownOpen} onOpenChange={setDropdownOpen}>
					<DropdownMenuTrigger asChild>
						<Button
							variant="outline"
							size="icon"
							className="shrink-0 w-10 mt-1"
							disabled={availableTags.length === 0}
						>
							<PlusIcon className="h-4 w-4" />
						</Button>
					</DropdownMenuTrigger>
					<DropdownMenuContent align="end" className="w-64 p-0 max-h-80">
						<Command>
							<CommandInput placeholder="Search tags..." />
							<CommandList className="max-h-64">
								<CommandEmpty>No tags found.</CommandEmpty>
								{availableTags.length > 0 && (
									<CommandGroup heading="Existing Tags">
										{availableTags.map((tag) => (
											<CommandItem
												key={tag}
												onSelect={() => {
													addTag(tag)
													setDropdownOpen(false)
												}}
											>
												{tag}
											</CommandItem>
										))}
									</CommandGroup>
								)}
							</CommandList>
						</Command>
					</DropdownMenuContent>
				</DropdownMenu>
			</div>
		)
	}
)

InputTags.displayName = "InputTags"

export { InputTags }
