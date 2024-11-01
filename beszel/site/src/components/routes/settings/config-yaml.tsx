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
import { useTranslation } from "react-i18next"

export default function ConfigYaml() {
	const { t } = useTranslation()

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
				title: "Error",
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
				<h3 className="text-xl font-medium mb-2">{t("settings.yaml_config.title")}</h3>
				<p className="text-sm text-muted-foreground leading-relaxed">{t("settings.yaml_config.subtitle")}</p>
			</div>
			<Separator className="my-4" />
			<div className="space-y-2">
				<div className="mb-4">
					<p className="text-sm text-muted-foreground leading-relaxed my-1">
						{t("settings.yaml_config.des_1")} <code className="bg-muted rounded-sm px-1 text-primary">config.yml</code>{" "}
						{t("settings.yaml_config.des_2")}
					</p>
					<p className="text-sm text-muted-foreground leading-relaxed">{t("settings.yaml_config.des_3")}</p>
					<Alert className="my-4 border-destructive text-destructive w-auto table md:pe-6">
						<AlertCircleIcon className="h-4 w-4 stroke-destructive" />
						<AlertTitle>{t("settings.yaml_config.alert.title")}</AlertTitle>
						<AlertDescription>
							<p>
								{t("settings.yaml_config.alert.des_1")} <code>config.yml</code> {t("settings.yaml_config.alert.des_2")}
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
				{t("settings.export_configuration")}
			</Button>
		</div>
	)
}
