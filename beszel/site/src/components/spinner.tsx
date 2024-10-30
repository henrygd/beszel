import { LoaderCircleIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'

export default function (props: { empty?: boolean }) {
	const { t } = useTranslation()
	return (
		<div className="flex flex-col items-center justify-center h-full absolute inset-0">
			<LoaderCircleIcon className="animate-spin h-10 w-10 opacity-60" />
			{props.empty && <p className={'opacity-60 mt-2'}>{t('monitor.waiting_for')}</p>}
		</div>
	)
}
