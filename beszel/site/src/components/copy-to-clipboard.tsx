import { useEffect, useMemo, useRef } from "react"
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "./ui/dialog"
import { Textarea } from "./ui/textarea"
import { $copyContent } from "@/lib/stores"
import { Trans } from "@lingui/macro"

export default function CopyToClipboard({ content }: { content: string }) {
	return (
		<Dialog defaultOpen={true}>
			<DialogContent className="w-[90%] rounded-lg md:pt-4" style={{ maxWidth: 530 }}>
				<DialogHeader>
					<DialogTitle>
						<Trans>Copy text</Trans>
					</DialogTitle>
					<DialogDescription className="hidden xs:block">
						<Trans>Automatic copy requires a secure context.</Trans>
					</DialogDescription>
				</DialogHeader>
				<CopyTextarea content={content} />
			</DialogContent>
		</Dialog>
	)
}

function CopyTextarea({ content }: { content: string }) {
	const textareaRef = useRef<HTMLTextAreaElement>(null)

	const rows = useMemo(() => {
		return content.split("\n").length
	}, [content])

	useEffect(() => {
		if (textareaRef.current) {
			textareaRef.current.select()
		}
	}, [textareaRef])

	useEffect(() => {
		return () => $copyContent.set("")
	}, [])

	return (
		<Textarea
			className="font-mono overflow-hidden whitespace-pre"
			rows={rows}
			value={content}
			readOnly
			ref={textareaRef}
		/>
	)
}
