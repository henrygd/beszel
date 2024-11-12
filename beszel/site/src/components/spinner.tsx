import { cn } from "@/lib/utils"
import { LoaderCircleIcon } from "lucide-react"

export default function ({ msg, className }: { msg?: string; className?: string }) {
	return (
		<div className={cn(className, "flex flex-col items-center justify-center h-full absolute inset-0")}>
			{msg ? (
				<p className={"opacity-60 mb-2 text-center text-sm px-4"}>{msg}</p>
			) : (
				<LoaderCircleIcon className="animate-spin h-10 w-10 opacity-60" />
			)}
		</div>
	)
}
