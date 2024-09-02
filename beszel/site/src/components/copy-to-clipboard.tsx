import { useEffect, useMemo, useRef } from 'react'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from './ui/dialog'
import { Textarea } from './ui/textarea'
import { $copyContent } from '@/lib/stores'

export default function CopyToClipboard({ content }: { content: string }) {
	return (
		<Dialog defaultOpen={true}>
			<DialogContent className="w-[90%] rounded-lg" style={{ maxWidth: 530 }}>
				<DialogHeader>
					<DialogTitle>Could not copy to clipboard</DialogTitle>
					<DialogDescription>Please copy the text manually.</DialogDescription>
				</DialogHeader>
				<CopyTextarea content={content} />
				<p className="text-sm text-muted-foreground">
					Clipboard API requires a secure context (https, localhost, or *.localhost)
				</p>
			</DialogContent>
		</Dialog>
	)
}

function CopyTextarea({ content }: { content: string }) {
	const textareaRef = useRef<HTMLTextAreaElement>(null)

	const rows = useMemo(() => {
		return content.split('\n').length
	}, [content])

	useEffect(() => {
		if (textareaRef.current) {
			textareaRef.current.select()
		}
	}, [textareaRef])

	useEffect(() => {
		return () => $copyContent.set('')
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
