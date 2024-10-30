import { useEffect, useMemo, useRef } from "react"
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "./ui/dialog"
import { Textarea } from "./ui/textarea"
import { $copyContent } from "@/lib/stores"
import { useTranslation } from "react-i18next"

export default function CopyToClipboard({ content }: { content: string }) {
	const { t } = useTranslation()

	return (
		<Dialog defaultOpen={true}>
			<DialogContent className="w-[90%] rounded-lg md:pt-4" style={{ maxWidth: 530 }}>
				<DialogHeader>
					<DialogTitle>{t("clipboard.title")}</DialogTitle>
					<DialogDescription className="hidden xs:block">{t("clipboard.des")}</DialogDescription>
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
