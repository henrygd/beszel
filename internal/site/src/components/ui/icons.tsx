import type { SVGProps } from "react"

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

// icon park (Apache 2.0) https://github.com/bytedance/IconPark/blob/master/LICENSE
export function WindowsIcon(props: SVGProps<SVGSVGElement>) {
	return (
		<svg {...props} viewBox="0 0 48 48">
			<path
				fill="none"
				stroke="currentColor"
				strokeWidth="3.8"
				d="m6.8 11 12.9-1.7v12.1h-13zm18-2.2 16.4-2v14.6H25zm0 18.6 16.4.4v13.4L25 38.6zm-18-.8 12.9.3v10.9l-13-2.2z"
			/>
		</svg>
	)
}

// teenyicons (MIT) https://github.com/teenyicons/teenyicons/blob/master/LICENSE
export function AppleIcon(props: SVGProps<SVGSVGElement>) {
	return (
		<svg viewBox="0 0 20 20" {...props}>
			<path
				fill="currentColor"
				d="M14.1 4.7a5 5 0 0 1 3.8 2c-3.3 1.9-2.8 6.7.6 8L17.2 17c-.8 1.3-2 2.9-3.5 2.9-1.2 0-1.6-.9-3.3-.8s-2.2.8-3.5.8c-1.4 0-2.5-1.5-3.4-2.7-2.3-3.6-2.5-7.9-1.1-10 1-1.7 2.6-2.6 4.1-2.6 1.6 0 2.6.8 3.8.8 1.3 0 2-.8 3.8-.8M13.7 0c.2 1.2-.3 2.4-1 3.2a4 4 0 0 1-3 1.6c-.2-1.2.3-2.3 1-3.2.7-.8 2-1.5 3-1.6"
			/>
		</svg>
	)
}

// Apache 2.0 https://github.com/Templarian/MaterialDesign/blob/master/LICENSE
export function FreeBsdIcon(props: SVGProps<SVGSVGElement>) {
	return (
		<svg viewBox="0 0 24 24" {...props}>
			<path
				fill="currentColor"
				d="M2.7 2C3.5 2 6 3.2 6 3.2 4.8 4 3.7 5 3 6.4 2.1 4.8 1.3 2.9 2 2.2l.7-.2m18.1.1c.4 0 .8 0 1 .2 1 1.1-2 5.8-2.4 6.4-.5.5-1.8 0-2.9-1-1-1.2-1.5-2.4-1-3 .4-.4 3.6-2.4 5.3-2.6m-8.8.5c1.3 0 2.5.2 3.7.7l-1 .7c-1 1-.6 2.8 1 4.4 1 1 2.1 1.6 3 1.6a2 2 0 0 0 1.5-.6l.7-1a9.7 9.7 0 1 1-18.6 3.8A9.7 9.7 0 0 1 12 2.7"
			/>
		</svg>
	)
}

// ion icons (MIT) https://github.com/ionic-team/ionicons/blob/main/LICENSE
export function DockerIcon(props: SVGProps<SVGSVGElement>) {
	return (
		<svg {...props} viewBox="0 0 512 512" fill="currentColor">
			<path d="M507 211c-1-1-14-11-42-11a133 133 0 0 0-21 2c-6-36-36-54-37-55l-7-4-5 7a102 102 0 0 0-13 30c-5 21-2 40 8 57-12 7-33 9-37 9H16a16 16 0 0 0-16 16 241 241 0 0 0 15 87c11 30 29 53 51 67 25 15 66 24 113 24a344 344 0 0 0 62-6 257 257 0 0 0 82-29 224 224 0 0 0 55-46c27-30 43-64 55-94h4c30 0 48-12 58-22a63 63 0 0 0 15-22l2-6Z" />
			<path d="M47 236h45a4 4 0 0 0 4-4v-40a4 4 0 0 0-4-4H47a4 4 0 0 0-4 4v40a4 4 0 0 0 4 4m63 0h45a4 4 0 0 0 4-4v-40a4 4 0 0 0-4-4h-45a4 4 0 0 0-4 4v40a4 4 0 0 0 4 4m63 0h45a4 4 0 0 0 4-4v-40a4 4 0 0 0-4-4h-45a4 4 0 0 0-4 4v40a4 4 0 0 0 4 4m62 0h45a4 4 0 0 0 4-4v-40a4 4 0 0 0-4-4h-45a4 4 0 0 0-4 4v40a4 4 0 0 0 4 4m-125-57h45a4 4 0 0 0 4-4v-41a4 4 0 0 0-4-4h-45a4 4 0 0 0-4 4v41a4 4 0 0 0 4 4m63 0h45a4 4 0 0 0 4-4v-41a4 4 0 0 0-4-4h-45a4 4 0 0 0-4 4v41a4 4 0 0 0 4 4m62 0h45a4 4 0 0 0 4-4v-41a4 4 0 0 0-4-4h-45a4 4 0 0 0-4 4v41a4 4 0 0 0 4 4m0-58h45a4 4 0 0 0 4-4V76a4 4 0 0 0-4-4h-45a4 4 0 0 0-4 4v40a4 4 0 0 0 4 4m63 116h45a4 4 0 0 0 4-4v-40a4 4 0 0 0-4-4h-45a4 4 0 0 0-4 4v40a4 4 0 0 0 4 4" />
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

// Remix icons (Apache 2.0) https://github.com/Remix-Design/RemixIcon/blob/master/License
export function HourglassIcon(props: SVGProps<SVGSVGElement>) {
	return (
		<svg viewBox="0 0 24 24" {...props} fill="currentColor">
			<path d="M4 2h16v4.5L13.5 12l6.5 5.5V22H4v-4.5l6.5-5.5L4 6.5zm12.3 5L18 5.5V4H6v1.5L7.7 7zM12 13.3l-6 5.2V20h1l5-3 5 3h1v-1.5z" />
		</svg>
	)
}

// Apache 2.0 https://github.com/Templarian/MaterialDesign/blob/master/LICENSE
export function WebSocketIcon(props: SVGProps<SVGSVGElement>) {
	return (
		<svg viewBox="0 0 256 193" {...props} fill="currentColor">
			<title>WebSocket</title>
			<path d="M192 145h32V68l-36-35-22 22 26 27zm32 16H113l-26-27 11-11 22 22h45l-44-45 11-11 44 44V88l-21-22 11-11-55-55H0l32 32h65l24 23-34 34-24-23V48H32v31l55 55-23 22 36 36h156z" />
		</svg>
	)
}

// Apache 2.0 https://github.com/Templarian/MaterialDesign/blob/master/LICENSE
export function BatteryMediumIcon(props: SVGProps<SVGSVGElement>) {
	return (
		<svg viewBox="0 0 24 24" {...props} fill="currentColor">
			<path d="M16 13H8V6h8m.67-2H15V2H9v2H7.33A1.33 1.33 0 0 0 6 5.33v15.34C6 21.4 6.6 22 7.33 22h9.34A1.33 1.33 0 0 0 18 20.67V5.33C18 4.6 17.4 4 16.67 4" />
		</svg>
	)
}

// Apache 2.0 https://github.com/Templarian/MaterialDesign/blob/master/LICENSE
export function BatteryLowIcon(props: SVGProps<SVGSVGElement>) {
	return (
		<svg viewBox="0 0 24 24" {...props} fill="currentColor">
			<path d="M16 17H8V6h8m.7-2H15V2H9v2H7.3A1.3 1.3 0 0 0 6 5.3v15.4q.1 1.2 1.3 1.3h9.4a1.3 1.3 0 0 0 1.3-1.3V5.3q-.1-1.2-1.3-1.3" />
		</svg>
	)
}

// Apache 2.0 https://github.com/Templarian/MaterialDesign/blob/master/LICENSE
export function BatteryHighIcon(props: SVGProps<SVGSVGElement>) {
	return (
		<svg viewBox="0 0 24 24" {...props} fill="currentColor">
			<path d="M16 9H8V6h8m.67-2H15V2H9v2H7.33A1.33 1.33 0 0 0 6 5.33v15.34C6 21.4 6.6 22 7.33 22h9.34A1.33 1.33 0 0 0 18 20.67V5.33C18 4.6 17.4 4 16.67 4" />
		</svg>
	)
}

// Apache 2.0 https://github.com/Templarian/MaterialDesign/blob/master/LICENSE
export function BatteryFullIcon(props: SVGProps<SVGSVGElement>) {
	return (
		<svg viewBox="0 0 24 24" {...props} fill="currentColor">
			<path d="M16.67 4H15V2H9v2H7.33A1.33 1.33 0 0 0 6 5.33v15.34C6 21.4 6.6 22 7.33 22h9.34A1.33 1.33 0 0 0 18 20.67V5.33C18 4.6 17.4 4 16.67 4" />
		</svg>
	)
}

// https://github.com/phosphor-icons/core (MIT license)
export function PlugChargingIcon(props: SVGProps<SVGSVGElement>) {
	return (
		<svg viewBox="0 0 256 256" {...props} fill="currentColor">
			<path d="M224,48H180V16a12,12,0,0,0-24,0V48H100V16a12,12,0,0,0-24,0V48H32.55C24.4,48,20,54.18,20,60A12,12,0,0,0,32,72H44v92a44.05,44.05,0,0,0,44,44h28v32a12,12,0,0,0,24,0V208h28a44.05,44.05,0,0,0,44-44V72h12a12,12,0,0,0,0-24ZM188,164a20,20,0,0,1-20,20H88a20,20,0,0,1-20-20V72H188Zm-85.86-29.17a12,12,0,0,1-1.38-11l12-32a12,12,0,1,1,22.48,8.42L129.32,116H144a12,12,0,0,1,11.24,16.21l-12,32a12,12,0,0,1-22.48-8.42L126.68,140H112A12,12,0,0,1,102.14,134.83Z" />
		</svg>
	)
}
