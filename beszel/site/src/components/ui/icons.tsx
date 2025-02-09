import { SVGProps } from "react"

// linux-logo-bold from https://github.com/phosphor-icons/core (MIT license)
export function TuxIcon(props: SVGProps<SVGSVGElement>) {
	return (
		<svg viewBox="0 0 256 256" {...props}>
			<path
				fill="currentColor"
				d="M231 217a12 12 0 0 1-16-2c-2-1-35-44-35-127a52 52 0 1 0-104 0c0 83-33 126-35 127a12 12 0 0 1-18-14c0-1 29-39 29-113a76 76 0 1 1 152 0c0 74 29 112 29 113a12 12 0 0 1-2 16m-127-97a16 16 0 1 0-16-16 16 16 0 0 0 16 16m64-16a16 16 0 1 0-16 16 16 16 0 0 0 16-16m-73 51 28 12a12 12 0 0 0 10 0l28-12a12 12 0 0 0-10-22l-23 10-23-10a12 12 0 0 0-10 22m33 29a57 57 0 0 0-39 15 12 12 0 0 0 17 18 33 33 0 0 1 44 0 12 12 0 1 0 17-18 57 57 0 0 0-39-15"
			/>
		</svg>
	)
}

// MingCute Apache License 2.0 https://github.com/Richard9394/MingCute
export function Rows(props: SVGProps<SVGSVGElement>) {
	return (
		<svg viewBox="0 0 24 24" {...props}>
			<path
				fill="currentColor"
				d="M5 3a2 2 0 0 0-2 2v4a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2V5a2 2 0 0 0-2-2zm0 2h14v4H5zm0 8a2 2 0 0 0-2 2v4a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-4a2 2 0 0 0-2-2zm0 2h14v4H5z"
			/>
		</svg>
	)
}

// IconPark Apache License 2.0 https://github.com/bytedance/IconPark
export function ChartAverage(props: SVGProps<SVGSVGElement>) {
	return (
		<svg fill="none" viewBox="0 0 48 48" stroke="currentColor" {...props}>
			<path strokeWidth="3" d="M4 4v40h40" />
			<path strokeWidth="3" d="M10 38S15.3 4 27 4s17 34 17 34" />
			<path strokeWidth="4" d="M10 24h34" />
		</svg>
	)
}

// IconPark Apache License 2.0 https://github.com/bytedance/IconPark
export function ChartMax(props: SVGProps<SVGSVGElement>) {
	return (
		<svg fill="none" viewBox="0 0 48 48" stroke="currentColor" {...props}>
			<path strokeWidth="3" d="M4 4v40h40" />
			<path strokeWidth="3" d="M10 38S15.3 4 27 4s17 34 17 34" />
			<path strokeWidth="4" d="M10 4h34" />
		</svg>
	)
}

// Lucide https://github.com/lucide-icons/lucide (not in package for some reason)
export function EthernetIcon(props: SVGProps<SVGSVGElement>) {
	return (
		<svg fill="none" stroke="currentColor" strokeLinecap="round" strokeWidth="2" viewBox="0 0 24 24" {...props}>
			<path d="m15 20 3-3h2a2 2 0 0 0 2-2V6a2 2 0 0 0-2-2H4a2 2 0 0 0-2 2v9a2 2 0 0 0 2 2h2l3 3zM6 8v1m4-1v1m4-1v1m4-1v1" />
		</svg>
	)
}

// Phosphor MIT https://github.com/phosphor-icons/core
export function ThermometerIcon(props: SVGProps<SVGSVGElement>) {
	return (
		<svg viewBox="0 0 256 256" {...props} fill="currentColor">
			<path d="M212 56a28 28 0 1 0 28 28 28 28 0 0 0-28-28m0 40a12 12 0 1 1 12-12 12 12 0 0 1-12 12m-60 50V40a32 32 0 0 0-64 0v106a56 56 0 1 0 64 0m-16-42h-32V40a16 16 0 0 1 32 0Z" />
		</svg>
	)
}

// Huge icons (MIT)
export function GpuIcon(props: SVGProps<SVGSVGElement>) {
	return (
		<svg viewBox="0 0 24 24" {...props} stroke="currentColor" fill="none" strokeWidth="2">
			<path d="M4 21V4.1a1.5 1.5 0 0 0-1.1-1L2 3m2 2h13c2.4 0 3.5 0 4.3.7s.7 2 .7 4.3v4.5c0 2.4 0 3.5-.7 4.3-.8.7-2 .7-4.3.7h-4.9a1.8 1.8 0 0 1-1.6-1c-.3-.6-1-1-1.6-1H4" />
			<path d="M19 11.5a3 3 0 1 1-6 0 3 3 0 0 1 6 0m-11.5-3h2m-2 3h2m-2 3h2" />
		</svg>
	)
}
