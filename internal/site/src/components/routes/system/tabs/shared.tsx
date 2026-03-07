import { t } from "@lingui/core/macro"
import { Trans, useLingui } from "@lingui/react/macro"
import { useStore } from "@nanostores/react"
import { XIcon } from "lucide-react"
import React, { memo, useCallback, useEffect, useState, type JSX } from "react"
import { $containerFilter, $maxValues } from "@/lib/stores"
import { useIntersectionObserver } from "@/lib/use-intersection-observer"
import { cn } from "@/lib/utils"
import Spinner from "../../../spinner"
import { Button } from "../../../ui/button"
import { Card, CardDescription, CardHeader, CardTitle } from "../../../ui/card"
import { ChartAverage, ChartMax } from "../../../ui/icons"
import { Input } from "../../../ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "../../../ui/select"

export function ChartCard({
	title,
	description,
	children,
	grid,
	empty,
	cornerEl,
	legend,
	className,
}: {
	title: string
	description: string
	children: React.ReactNode
	grid?: boolean
	empty?: boolean
	cornerEl?: JSX.Element | null
	legend?: boolean
	className?: string
}) {
	const { isIntersecting, ref } = useIntersectionObserver()

	return (
		<Card
			className={cn("pb-2 sm:pb-4 odd:last-of-type:col-span-full min-h-full", { "col-span-full": !grid }, className)}
			ref={ref}
		>
			<CardHeader className="pb-5 pt-4 gap-1 relative max-sm:py-3 max-sm:px-4">
				<CardTitle className="text-xl sm:text-2xl">{title}</CardTitle>
				<CardDescription>{description}</CardDescription>
				{cornerEl && <div className="py-1 grid sm:justify-end sm:absolute sm:top-3.5 sm:end-3.5">{cornerEl}</div>}
			</CardHeader>
			<div className={cn("ps-0 w-[calc(100%-1.3em)] relative group", legend ? "h-54 md:h-56" : "h-48 md:h-52")}>
				{
					<Spinner
						msg={empty ? t`Waiting for enough records to display` : undefined}
						className="group-has-[.opacity-100]:invisible duration-100"
					/>
				}
				{isIntersecting && children}
			</div>
		</Card>
	)
}

export function FilterBar({ store = $containerFilter }: { store?: typeof $containerFilter }) {
	const storeValue = useStore(store)
	const [inputValue, setInputValue] = useState(storeValue)
	const { t } = useLingui()

	useEffect(() => {
		setInputValue(storeValue)
	}, [storeValue])

	useEffect(() => {
		if (inputValue === storeValue) {
			return
		}
		const handle = window.setTimeout(() => store.set(inputValue), 80)
		return () => clearTimeout(handle)
	}, [inputValue, storeValue, store])

	const handleChange = useCallback(
		(e: React.ChangeEvent<HTMLInputElement>) => {
			const value = e.target.value
			setInputValue(value)
		},
		[]
	)

	const handleClear = useCallback(() => {
		setInputValue("")
		store.set("")
	}, [store])

	return (
		<>
			<Input
				placeholder={t`Filter...`}
				className="ps-4 pe-8 w-full sm:w-44"
				onChange={handleChange}
				value={inputValue}
			/>
			{inputValue && (
				<Button
					type="button"
					variant="ghost"
					size="icon"
					aria-label="Clear"
					className="absolute right-1 top-1/2 -translate-y-1/2 h-7 w-7 text-gray-500 hover:text-gray-900 dark:text-gray-400 dark:hover:text-gray-100"
					onClick={handleClear}
				>
					<XIcon className="h-4 w-4" />
				</Button>
			)}
		</>
	)
}

export const SelectAvgMax = memo(({ max }: { max: boolean }) => {
	const Icon = max ? ChartMax : ChartAverage
	return (
		<Select value={max ? "max" : "avg"} onValueChange={(e) => $maxValues.set(e === "max")}>
			<SelectTrigger className="relative ps-10 pe-5 w-full sm:w-44">
				<Icon className="h-4 w-4 absolute start-4 top-1/2 -translate-y-1/2 opacity-85" />
				<SelectValue />
			</SelectTrigger>
			<SelectContent>
				<SelectItem key="avg" value="avg">
					<Trans>Average</Trans>
				</SelectItem>
				<SelectItem key="max" value="max">
					<Trans comment="Chart select field. Please try to keep this short.">Max 1 min</Trans>
				</SelectItem>
			</SelectContent>
		</Select>
	)
})
