import * as React from "react"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { XIcon } from "lucide-react"
import { type InputProps } from "./input"
import { cn } from "@/lib/utils"

type InputTagsProps = Omit<InputProps, "value" | "onChange"> & {
	value: string[]
	onChange: React.Dispatch<React.SetStateAction<string[]>>
}

const InputTags = React.forwardRef<HTMLInputElement, InputTagsProps>(
	({ className, value, onChange, ...props }, ref) => {
		const [pendingDataPoint, setPendingDataPoint] = React.useState("")

		React.useEffect(() => {
			if (pendingDataPoint.includes(",")) {
				const newDataPoints = new Set([...value, ...pendingDataPoint.split(",").map((chunk) => chunk.trim())])
				onChange(Array.from(newDataPoints))
				setPendingDataPoint("")
			}
		}, [pendingDataPoint, onChange, value])

		const addPendingDataPoint = () => {
			if (pendingDataPoint) {
				const newDataPoints = new Set([...value, pendingDataPoint])
				onChange(Array.from(newDataPoints))
				setPendingDataPoint("")
			}
		}

		return (
			<div
				className={cn(
					"bg-background min-h-10 flex w-full flex-wrap gap-2 rounded-md border px-3 py-2 text-sm  placeholder:text-muted-foreground has-[:focus-visible]:outline-none ring-offset-background has-[:focus-visible]:ring-2 has-[:focus-visible]:ring-ring has-[:focus-visible]:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50",
					className
				)}
			>
				{value.map((item) => (
					<Badge key={item}>
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
					className="flex-1 outline-none bg-background placeholder:text-muted-foreground"
					value={pendingDataPoint}
					onChange={(e) => setPendingDataPoint(e.target.value)}
					onKeyDown={(e) => {
						if (e.key === "Enter" || e.key === ",") {
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
		)
	}
)

InputTags.displayName = "InputTags"

export { InputTags }
