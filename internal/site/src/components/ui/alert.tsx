import { cva, type VariantProps } from "class-variance-authority"
import * as React from "react"

import { cn } from "@/lib/utils"

const alertVariants = cva(
	"relative w-full rounded-lg border p-4 [&>svg~*]:ps-7 [&>svg+div]:translate-y-[-3px] [&>svg]:absolute [&>svg]:left-4 [&>svg]:top-4",
	{
		variants: {
			variant: {
				default: "bg-background text-foreground [&>svg]:text-foreground",
				destructive:
					"bg-destructive/5 border-destructive/50 text-destructive dark:bg-destructive/6 dark:text-red-400 dark:border-destructive/50 [&>svg]:text-destructive dark:[&>svg]:text-red-400",
			},
		},
		defaultVariants: {
			variant: "default",
		},
	}
)

const Alert = React.forwardRef<
	HTMLDivElement,
	React.HTMLAttributes<HTMLDivElement> & VariantProps<typeof alertVariants>
>(({ className, variant, ...props }, ref) => (
	<div ref={ref} role="alert" className={cn(alertVariants({ variant }), className)} {...props} />
))
Alert.displayName = "Alert"

const AlertTitle = React.forwardRef<HTMLParagraphElement, React.HTMLAttributes<HTMLHeadingElement>>(
	({ className, ...props }, ref) => (
		<h5 ref={ref} className={cn("mb-1 -mt-0.5 font-medium leading-tight tracking-tight", className)} {...props} />
	)
)
AlertTitle.displayName = "AlertTitle"

const AlertDescription = React.forwardRef<HTMLParagraphElement, React.HTMLAttributes<HTMLParagraphElement>>(
	({ className, ...props }, ref) => (
		<div ref={ref} className={cn("text-sm [&_p]:leading-relaxed", className)} {...props} />
	)
)
AlertDescription.displayName = "AlertDescription"

export { Alert, AlertTitle, AlertDescription }
