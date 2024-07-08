import { useEffect } from 'preact/hooks'
import { useRoute } from 'wouter-preact'

export function ServerDetail() {
	const [_, params] = useRoute('/server/:name')

	useEffect(() => {
		document.title = `Server: ${params!.name}`
	}, [])

	return <>Info for {params!.name}</>
}
