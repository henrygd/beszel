import { Button } from '@/components/ui/button'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip'

export function TooltipDemo() {
	return (
		<TooltipProvider delayDuration={0}>
			<Tooltip>
				<TooltipTrigger>
					<Button
						onMouseEnter={() => console.log('hovered')}
						onMouseLeave={() => console.log('unhovered')}
						variant="outline"
					>
						Hover
					</Button>
				</TooltipTrigger>
				<TooltipContent hideWhenDetached={true} className="pointer-events-none">
					<p>Add to library</p>
				</TooltipContent>
			</Tooltip>
		</TooltipProvider>
	)
}
