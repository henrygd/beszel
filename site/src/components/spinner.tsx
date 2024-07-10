import { LoaderCircleIcon } from 'lucide-react'

export default function () {
	return (
		<div className="grid place-content-center h-full absolute inset-0">
			<LoaderCircleIcon className="animate-spin h-10 w-10 opacity-60" />
		</div>
	)
}
