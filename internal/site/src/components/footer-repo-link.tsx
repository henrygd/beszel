import { useStore } from "@nanostores/react"
import { GithubIcon } from "lucide-react"
import { $newVersion } from "@/lib/stores"
import { Separator } from "./ui/separator"
import { Trans } from "@lingui/react/macro"

export function FooterRepoLink() {
	const newVersion = useStore($newVersion)
	return (
		<div className="flex gap-1.5 justify-end items-center pe-3 sm:pe-6 mt-3.5 mb-4 text-xs opacity-80">
			<a
				href="https://github.com/henrygd/beszel"
				target="_blank"
				className="flex items-center gap-0.5 text-muted-foreground hover:text-foreground duration-75"
				rel="noopener"
			>
				<GithubIcon className="h-3 w-3" /> GitHub
			</a>
			<Separator orientation="vertical" className="h-2.5 bg-muted-foreground opacity-70" />
			<a
				href="https://github.com/henrygd/beszel/releases"
				target="_blank"
				className="text-muted-foreground hover:text-foreground duration-75"
				rel="noopener"
			>
				Beszel {globalThis.BESZEL.HUB_VERSION}
			</a>
			{newVersion?.v && (
				<>
					<Separator orientation="vertical" className="h-2.5 bg-muted-foreground opacity-70" />
					<a
						href={newVersion.url}
						target="_blank"
						className="text-yellow-500 hover:text-yellow-400 duration-75"
						rel="noopener"
					>
						<Trans context="New version available">{newVersion.v} available</Trans>
					</a>
				</>
			)}
		</div>
	)
}
