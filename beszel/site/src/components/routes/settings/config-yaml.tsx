import { isAdmin } from "@/lib/utils"
import { Separator } from "@/components/ui/separator"
import { Button } from "@/components/ui/button"
import { redirectPage } from "@nanostores/router"
import { $router } from "@/components/router"
import { AlertCircleIcon, FileSlidersIcon, LoaderCircleIcon } from "lucide-react"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { pb } from "@/lib/stores"
import { useState } from "react"
import { Textarea } from "@/components/ui/textarea"
import { toast } from "@/components/ui/use-toast"
import clsx from "clsx"
import { Trans, t } from "@lingui/macro"

export default function ConfigYaml() {
	const [configContent, setConfigContent] = useState<string>("")
	const [isLoading, setIsLoading] = useState(false)

	const ButtonIcon = isLoading ? LoaderCircleIcon : FileSlidersIcon

	async function fetchConfig() {
		try {
			setIsLoading(true)
			const { config } = await pb.send<{ config: string }>("/api/beszel/config-yaml", {})
			setConfigContent(config)
		} catch (error: any) {
			toast({
				title: t`Error`,
				description: error.message,
				variant: "destructive",
			})
		} finally {
			setIsLoading(false)
		}
	}

	if (!isAdmin()) {
		redirectPage($router, "settings", { name: "general" })
	}

	return (
		<div>
			<div>
				<h3 className="text-xl font-medium mb-2">
					<Trans>YAML Configuration</Trans>
				</h3>
				<p className="text-sm text-muted-foreground leading-relaxed">
					<Trans>Export your current systems configuration.</Trans>
				</p>
			</div>
			<Separator className="my-4" />
			<div className="space-y-2">
				<div className="mb-4">
					<p className="text-sm text-muted-foreground leading-relaxed my-1">
						<Trans>
							Systems may be managed in a <code className="bg-muted rounded-sm px-1 text-primary">config.yml</code> file
							inside your data directory.
						</Trans>
					</p>
					<p className="text-sm text-muted-foreground leading-relaxed">
						<Trans>
							On each restart, systems in the database will be updated to match the systems defined in the file.
						</Trans>
					</p>
					<Alert className="my-4 border-destructive text-destructive w-auto table md:pe-6">
						<AlertCircleIcon className="h-4 w-4 stroke-destructive" />
						<AlertTitle>
							<Trans>Caution - potential data loss</Trans>
						</AlertTitle>
						<AlertDescription>
							<p>
								<Trans>
									Existing systems not defined in <code>config.yml</code> will be deleted. Please make regular backups.
								</Trans>
							</p>
						</AlertDescription>
					</Alert>
				</div>
				{configContent && (
					<Textarea
						dir="ltr"
						autoFocus
						defaultValue={configContent}
						spellCheck="false"
						rows={Math.min(25, configContent.split("\n").length)}
						className="font-mono whitespace-pre"
					/>
				)}
			</div>
			<Separator className="my-5" />
			<Button type="button" className="mt-2 flex items-center gap-1" onClick={fetchConfig} disabled={isLoading}>
				<ButtonIcon className={clsx("h-4 w-4 me-0.5", isLoading && "animate-spin")} />
				<Trans>Export configuration</Trans>
			</Button>
		</div>
	)
}
