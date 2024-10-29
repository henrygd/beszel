import { useEffect } from 'react'
import { GlobeIcon, Languages } from 'lucide-react'

import { Button } from '@/components/ui/button'
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { useTranslation } from 'react-i18next'
import languages from '../lib/languages.json'

export function LangToggle() {
	const { i18n } = useTranslation();

	useEffect(() => {
		document.documentElement.lang = i18n.language;
	}, [i18n.language]);

	return (
		<DropdownMenu>
			<DropdownMenuTrigger asChild>
				<Button variant={'ghost'} size="icon">
					<GlobeIcon className="absolute h-[1.2rem] w-[1.2rem]" />
					<span className="sr-only">Language</span>
				</Button>
			</DropdownMenuTrigger>
			<DropdownMenuContent>
				{languages.map(({ lang, label }) => (
					<DropdownMenuItem
						key={lang}
						className={lang === i18n.language ? 'font-bold' : ''}
						onClick={() => i18n.changeLanguage(lang)}
					>
						{label}
					</DropdownMenuItem>
				))}
			</DropdownMenuContent>
		</DropdownMenu>
	)
}
