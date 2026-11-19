"use client"

import { useState, useEffect } from "react"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { zodResolver } from "@hookform/resolvers/zod"
import { useForm } from "react-hook-form"
import { z } from "zod"
import { useToast } from "@/components/ui/use-toast"
import { Button } from "@/components/ui/button"
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "@/components/ui/dialog"
import {
	Form,
	FormControl,
	FormField,
	FormItem,
	FormLabel,
	FormMessage,
} from "@/components/ui/form"
import { Input } from "@/components/ui/input"
import { Switch } from "@/components/ui/switch"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import {
	createDomain,
	updateDomain,
	lookupDomain,
	cleanDomain,
	type Domain,
	type CreateDomainRequest,
	type UpdateDomainRequest,
	type DomainLookupResult,
} from "@/lib/domains"
import { Loader2, Search, AlertTriangle, Calendar } from "lucide-react"

const formSchema = z.object({
	domain_name: z.string().min(1, "Domain name is required"),
	tags: z.string().optional(),
	notes: z.string().optional(),
	purchase_price: z.coerce.number().min(0).optional(),
	current_value: z.coerce.number().min(0).optional(),
	renewal_cost: z.coerce.number().min(0).optional(),
	auto_renew: z.boolean(),
	monitor_type: z.enum(["expiry", "watchlist", "portfolio"]),
	// Expiry alerts
	alert_days_before: z.coerce.number().min(1).max(365),
	ssl_alert_enabled: z.boolean(),
	ssl_alert_days: z.coerce.number().min(1).max(90),
	// Notification settings
	notify_on_expiry: z.boolean(),
	notify_on_ssl_expiry: z.boolean(),
	notify_on_dns_change: z.boolean(),
	notify_on_registrar_change: z.boolean(),
	notify_on_value_change: z.boolean(),
	value_change_threshold: z.coerce.number().min(1).optional(),
	// Quiet hours
	quiet_hours_enabled: z.boolean(),
	quiet_hours_start: z.string(),
	quiet_hours_end: z.string(),
})

type FormData = z.infer<typeof formSchema>

interface DomainDialogProps {
	open: boolean
	onOpenChange: (open: boolean) => void
	domain?: Domain | null
	isEdit?: boolean
}

export function DomainDialog({ open, onOpenChange, domain, isEdit = false }: DomainDialogProps) {
	const { toast } = useToast()
	const queryClient = useQueryClient()
	const [activeTab, setActiveTab] = useState("basic")
	const [lookupData, setLookupData] = useState<DomainLookupResult | null>(null)
	const [isLookingUp, setIsLookingUp] = useState(false)
	// Manual expiry inputs when WHOIS fails
	const [manualRegDate, setManualRegDate] = useState(() => {
		const today = new Date()
		return today.toISOString().split("T")[0]
	})
	const [manualRegPeriod, setManualRegPeriod] = useState<number>(1)
	const [manualPurchasePrice, setManualPurchasePrice] = useState<number>(0)

	const form = useForm<z.input<typeof formSchema>, any, z.output<typeof formSchema>>({
		resolver: zodResolver(formSchema),
		defaultValues: {
			domain_name: "",
			tags: "",
			notes: "",
			purchase_price: 0,
			current_value: 0,
			renewal_cost: 0,
			auto_renew: false,
			monitor_type: "expiry",
			// Expiry alerts
			alert_days_before: 30,
			ssl_alert_enabled: true,
			ssl_alert_days: 14,
			// Notification settings
			notify_on_expiry: true,
			notify_on_ssl_expiry: true,
			notify_on_dns_change: false,
			notify_on_registrar_change: false,
			notify_on_value_change: false,
			value_change_threshold: 10,
			// Quiet hours
			quiet_hours_enabled: false,
			quiet_hours_start: "22:00",
			quiet_hours_end: "08:00",
		},
	})

	useEffect(() => {
		if (open && isEdit && domain) {
			form.reset({
				domain_name: domain.domain_name,
				tags: domain.tags?.join(", ") || "",
				notes: domain.notes || "",
				purchase_price: domain.purchase_price || 0,
				current_value: domain.current_value || 0,
				renewal_cost: domain.renewal_cost || 0,
				auto_renew: domain.auto_renew || false,
				monitor_type: domain.monitor_type || "expiry",
				// Expiry alerts
				alert_days_before: domain.alert_days_before || 30,
				ssl_alert_enabled: domain.ssl_alert_enabled || true,
				ssl_alert_days: domain.ssl_alert_days || 14,
				// Notification settings
				notify_on_expiry: domain.notify_on_expiry !== false,
				notify_on_ssl_expiry: domain.notify_on_ssl_expiry !== false,
				notify_on_dns_change: domain.notify_on_dns_change || false,
				notify_on_registrar_change: domain.notify_on_registrar_change || false,
				notify_on_value_change: domain.notify_on_value_change || false,
				value_change_threshold: domain.value_change_threshold || 10,
				// Quiet hours
				quiet_hours_enabled: domain.quiet_hours_enabled || false,
				quiet_hours_start: domain.quiet_hours_start || "22:00",
				quiet_hours_end: domain.quiet_hours_end || "08:00",
			})
		} else if (open && !isEdit) {
			form.reset({
				domain_name: "",
				tags: "",
				notes: "",
				purchase_price: 0,
				current_value: 0,
				renewal_cost: 0,
				auto_renew: false,
				monitor_type: "expiry",
				// Expiry alerts
				alert_days_before: 30,
				ssl_alert_enabled: true,
				ssl_alert_days: 14,
				// Notification settings
				notify_on_expiry: true,
				notify_on_ssl_expiry: true,
				notify_on_dns_change: false,
				notify_on_registrar_change: false,
				notify_on_value_change: false,
				value_change_threshold: 10,
				// Quiet hours
				quiet_hours_enabled: false,
				quiet_hours_start: "22:00",
				quiet_hours_end: "08:00",
			})
			setLookupData(null)
			const today = new Date().toISOString().split("T")[0]
			setManualRegDate(today)
			setManualRegPeriod(1)
			setManualPurchasePrice(0)
		}
	}, [open, isEdit, domain, form])

	const createMutation = useMutation({
		mutationFn: createDomain,
		onSuccess: () => {
			toast({ title: "Domain added successfully" })
			queryClient.invalidateQueries({ queryKey: ["domains"] })
			onOpenChange(false)
		},
		onError: (error: Error) => {
			toast({
				title: "Failed to add domain",
				description: error.message,
				variant: "destructive",
			})
		},
	})

	const updateMutation = useMutation({
		mutationFn: ({ id, data }: { id: string; data: UpdateDomainRequest }) =>
			updateDomain(id, data),
		onSuccess: () => {
			toast({ title: "Domain updated successfully" })
			queryClient.invalidateQueries({ queryKey: ["domains"] })
			onOpenChange(false)
		},
		onError: (error: Error) => {
			toast({
				title: "Failed to update domain",
				description: error.message,
				variant: "destructive",
			})
		},
	})

	const handleLookup = async () => {
		const domainName = form.getValues("domain_name")
		if (!domainName) return

		setIsLookingUp(true)
		try {
			const data = await lookupDomain(domainName)
			setLookupData(data)
			// Reset manual inputs on new lookup
			const today = new Date().toISOString().split("T")[0]
			setManualRegDate(today)
			setManualRegPeriod(1)
			setManualPurchasePrice(0)
			toast({ title: "Domain info retrieved successfully" })
		} catch (error) {
			toast({
				title: "Failed to lookup domain",
				description: error instanceof Error ? error.message : "Unknown error",
				variant: "destructive",
			})
		} finally {
			setIsLookingUp(false)
		}
	}

	// Calculate expiry date from registration date + period in years
	const calculateExpiryDate = (regDateStr: string, years: number): string | null => {
		const regDate = new Date(regDateStr)
		if (isNaN(regDate.getTime())) return null
		const expiry = new Date(regDate)
		expiry.setFullYear(expiry.getFullYear() + years)
		// Subtract 1 day (expiry is typically the day before the anniversary)
		expiry.setDate(expiry.getDate() - 1)
		return expiry.toISOString().split("T")[0]
	}

	const onSubmit = (data: FormData) => {
		const payload: CreateDomainRequest = {
			domain_name: cleanDomain(data.domain_name),
			auto_lookup: !isEdit && lookupData !== null,
			tags: data.tags?.split(",").map((t) => t.trim()).filter(Boolean),
			notes: data.notes,
			purchase_price: data.purchase_price,
			current_value: data.current_value,
			renewal_cost: data.renewal_cost,
			auto_renew: data.auto_renew,
			monitor_type: data.monitor_type,
			// Expiry alerts
			alert_days_before: data.alert_days_before,
			ssl_alert_enabled: data.ssl_alert_enabled,
			ssl_alert_days: data.ssl_alert_days,
			// Notification settings
			notify_on_expiry: data.notify_on_expiry,
			notify_on_ssl_expiry: data.notify_on_ssl_expiry,
			notify_on_dns_change: data.notify_on_dns_change,
			notify_on_registrar_change: data.notify_on_registrar_change,
			notify_on_value_change: data.notify_on_value_change,
			value_change_threshold: data.notify_on_value_change ? data.value_change_threshold : undefined,
			// Quiet hours
			quiet_hours_enabled: data.quiet_hours_enabled,
			quiet_hours_start: data.quiet_hours_enabled ? data.quiet_hours_start : undefined,
			quiet_hours_end: data.quiet_hours_enabled ? data.quiet_hours_end : undefined,
		}

		// If lookup returned no expiry, attach manual dates
		if (!isEdit && lookupData && !lookupData.expiry_date) {
			const calculatedExpiry = calculateExpiryDate(manualRegDate, manualRegPeriod)
			if (calculatedExpiry) {
				payload.expiry_date = calculatedExpiry
				payload.creation_date = manualRegDate
			}
			// Use the manual purchase price if set (overrides form value when WHOIS fails)
			if (manualPurchasePrice > 0) {
				payload.purchase_price = manualPurchasePrice
			}
		}

		if (isEdit && domain) {
			updateMutation.mutate({
				id: domain.id,
				data: {
					tags: payload.tags,
					notes: payload.notes,
					purchase_price: payload.purchase_price,
					current_value: payload.current_value,
					renewal_cost: payload.renewal_cost,
					auto_renew: payload.auto_renew,
					monitor_type: payload.monitor_type,
					// Expiry alerts
					alert_days_before: payload.alert_days_before,
					ssl_alert_enabled: payload.ssl_alert_enabled,
					ssl_alert_days: payload.ssl_alert_days,
					// Notification settings
					notify_on_expiry: payload.notify_on_expiry,
					notify_on_ssl_expiry: payload.notify_on_ssl_expiry,
					notify_on_dns_change: payload.notify_on_dns_change,
					notify_on_registrar_change: payload.notify_on_registrar_change,
					notify_on_value_change: payload.notify_on_value_change,
					value_change_threshold: payload.value_change_threshold,
					// Quiet hours
					quiet_hours_enabled: payload.quiet_hours_enabled,
					quiet_hours_start: payload.quiet_hours_start,
					quiet_hours_end: payload.quiet_hours_end,
				},
			})
		} else {
			createMutation.mutate(payload)
		}
	}

	const isPending = createMutation.isPending || updateMutation.isPending

	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent className="sm:max-w-[600px] max-h-[80vh] overflow-y-auto">
				<DialogHeader>
					<DialogTitle>
						{isEdit ? "Edit Domain" : "Add Domain"}
					</DialogTitle>
					<DialogDescription>
						{isEdit
							? "Update domain tracking settings."
							: "Add a domain to track its expiry, SSL, and DNS information."}
					</DialogDescription>
				</DialogHeader>

				<Form {...form}>
					<form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">
						<Tabs value={activeTab} onValueChange={setActiveTab}>
							<TabsList className="grid w-full grid-cols-3">
								<TabsTrigger value="basic">Basic</TabsTrigger>
								<TabsTrigger value="valuation">Valuation</TabsTrigger>
								<TabsTrigger value="alerts">Alerts</TabsTrigger>
							</TabsList>

							<TabsContent value="basic" className="space-y-4 mt-4">
								{!isEdit && (
								<div className="flex gap-2 items-end">
									<FormField
										control={form.control}
										name="domain_name"
										render={({ field }) => (
											<FormItem className="flex-1">
												<FormLabel>Domain Name</FormLabel>
												<FormControl>
													<Input placeholder="example.com" autoFocus tabIndex={0} {...field} value={field.value as string | number | undefined} />
												</FormControl>
												<FormMessage />
											</FormItem>
										)}
									/>
									<Button
										type="button"
										variant="outline"
										onClick={handleLookup}
										disabled={isLookingUp || !form.getValues("domain_name")}
										className="shrink-0"
									>
										{isLookingUp ? (
											<Loader2 className="h-4 w-4 animate-spin" />
										) : (
											<>
												<Search className="h-4 w-4 mr-2" />
												<span>Lookup</span>
											</>
										)}
									</Button>
								</div>
							)}

								{isEdit && (
									<FormField
										control={form.control}
										name="domain_name"
										render={({ field }) => (
											<FormItem>
												<FormLabel>Domain Name</FormLabel>
												<FormControl>
													<Input disabled {...field} value={field.value as string | number | undefined} />
												</FormControl>
											</FormItem>
										)}
									/>
								)}

								<FormField
									control={form.control}
									name="tags"
									render={({ field }) => (
										<FormItem>
											<FormLabel>Tags (comma separated)</FormLabel>
											<FormControl>
												<Input placeholder="portfolio, client, investment" {...field} value={field.value as string | number | undefined} />
											</FormControl>
											<FormMessage />
										</FormItem>
									)}
								/>

								<FormField
									control={form.control}
									name="notes"
									render={({ field }) => (
										<FormItem>
											<FormLabel>Notes</FormLabel>
											<FormControl>
												<Input placeholder="Any additional information..." {...field} value={field.value as string | number | undefined} />
											</FormControl>
											<FormMessage />
										</FormItem>
									)}
								/>

								{lookupData && !isEdit && (
									<div className="space-y-3">
										{/* Lookup Results */}
										<div className="rounded-lg border p-4 space-y-2">
											<h4 className="font-medium">Lookup Results</h4>
											{lookupData.registrar_name && (
												<p className="text-sm">Registrar: {lookupData.registrar_name}</p>
											)}
											{lookupData.expiry_date && (
												<p className="text-sm">Expires: {lookupData.expiry_date}</p>
											)}
											{lookupData.ssl_valid_to && (
												<p className="text-sm">SSL Expires: {lookupData.ssl_valid_to}</p>
											)}
											{lookupData.host_country && (
												<p className="text-sm">Location: {lookupData.host_country}</p>
											)}
											{lookupData.dns_provider && (
												<p className="text-sm">DNS: {lookupData.dns_provider}</p>
											)}
											{lookupData.hosting_provider && (
												<p className="text-sm">Hosting: {lookupData.hosting_provider}</p>
											)}
											{lookupData.email_provider && (
												<p className="text-sm">Email: {lookupData.email_provider}</p>
											)}
											{lookupData.ca_provider && (
												<p className="text-sm">CA: {lookupData.ca_provider}</p>
											)}
										</div>

										{/* Manual expiry fallback when WHOIS doesn't return expiry */}
										{!lookupData.expiry_date && (
											<div className="rounded-lg border border-yellow-500/30 bg-yellow-500/5 p-4 space-y-3">
												<div className="flex items-center gap-2 text-yellow-700">
													<AlertTriangle className="h-4 w-4" />
													<h4 className="font-medium text-sm">Expiry date not found in WHOIS</h4>
												</div>
												<p className="text-xs text-muted-foreground">
													Enter your registration details below and we&apos;ll calculate the expiry date.
												</p>

												<div className="grid grid-cols-2 gap-3">
													<div className="space-y-1">
														<label className="text-xs font-medium">Registration Date</label>
														<div className="flex items-center gap-2">
															<Calendar className="h-3.5 w-3.5 text-muted-foreground" />
															<Input
																type="date"
																value={manualRegDate}
																onChange={(e) => setManualRegDate(e.target.value)}
																className="h-8 text-sm"
															/>
														</div>
													</div>
													<div className="space-y-1">
														<label className="text-xs font-medium">Registration Period</label>
														<select
															value={manualRegPeriod}
															onChange={(e) => setManualRegPeriod(Number(e.target.value))}
															className="w-full h-8 px-2 rounded-md border border-input bg-background text-sm"
														>
															<option value={1}>1 year</option>
															<option value={2}>2 years</option>
															<option value={3}>3 years</option>
															<option value={5}>5 years</option>
															<option value={10}>10 years</option>
														</select>
													</div>
												</div>

												<div className="space-y-1">
													<label className="text-xs font-medium">Purchase Price (total for selected period)</label>
													<Input
															type="number"
															min={0}
															step="0.01"
															value={manualPurchasePrice || ""}
															placeholder="e.g. 29.99"
															onChange={(e) => setManualPurchasePrice(Number(e.target.value))}
															className="h-8 text-sm"
														/>
														{manualPurchasePrice > 0 && manualRegPeriod > 1 && (
															<p className="text-[10px] text-muted-foreground">
																~{(manualPurchasePrice / manualRegPeriod).toFixed(2)} per year
															</p>
														)}
												</div>

												{manualRegDate && manualRegPeriod > 0 && (
													<div className="rounded-md bg-muted p-2 text-xs">
														<p className="font-medium">Calculated Expiry:</p>
														<p className="text-muted-foreground">
															{calculateExpiryDate(manualRegDate, manualRegPeriod)}
														</p>
													</div>
												)}
											</div>
										)}
									</div>
								)}
							</TabsContent>

							<TabsContent value="valuation" className="space-y-4 mt-4">
								<FormField
									control={form.control}
									name="purchase_price"
									render={({ field }) => (
										<FormItem>
											<FormLabel>Purchase Price</FormLabel>
											<FormControl>
												<Input type="number" {...field} value={field.value as string | number | undefined} />
											</FormControl>
											<FormMessage />
										</FormItem>
									)}
								/>

								<FormField
									control={form.control}
									name="current_value"
									render={({ field }) => (
										<FormItem>
											<FormLabel>Current Value</FormLabel>
											<FormControl>
												<Input type="number" {...field} value={field.value as string | number | undefined} />
											</FormControl>
											<FormMessage />
										</FormItem>
									)}
								/>

								<FormField
									control={form.control}
									name="renewal_cost"
									render={({ field }) => (
										<FormItem>
											<FormLabel>Renewal Cost</FormLabel>
											<FormControl>
												<Input type="number" {...field} value={field.value as string | number | undefined} />
											</FormControl>
											<FormMessage />
										</FormItem>
									)}
								/>

								<FormField
									control={form.control}
									name="auto_renew"
									render={({ field }) => (
										<FormItem className="flex items-center justify-between rounded-lg border p-3">
											<div className="space-y-0.5">
												<FormLabel>Auto Renew</FormLabel>
											</div>
											<FormControl>
												<Switch
													checked={field.value}
													onCheckedChange={field.onChange}
												/>
											</FormControl>
										</FormItem>
									)}
								/>
							</TabsContent>

							<TabsContent value="alerts" className="space-y-4 mt-4">
								{/* Monitor Type */}
								<FormField
									control={form.control}
									name="monitor_type"
									render={({ field }) => (
										<FormItem className="rounded-lg border p-4">
											<FormLabel className="font-medium">Monitoring Purpose</FormLabel>
											<FormControl>
												<div className="grid grid-cols-3 gap-2 mt-2">
													<Button
														type="button"
														variant={field.value === "expiry" ? "default" : "outline"}
														onClick={() => field.onChange("expiry")}
														className="flex-col h-auto py-3"
													>
														<span className="text-xs">Track Expiry</span>
														<span className="text-[10px] opacity-70">Monitor expiration</span>
													</Button>
													<Button
														type="button"
														variant={field.value === "watchlist" ? "default" : "outline"}
														onClick={() => field.onChange("watchlist")}
														className="flex-col h-auto py-3"
													>
														<span className="text-xs">Watch to Buy</span>
														<span className="text-[10px] opacity-70">Track availability</span>
													</Button>
													<Button
														type="button"
														variant={field.value === "portfolio" ? "default" : "outline"}
														onClick={() => field.onChange("portfolio")}
														className="flex-col h-auto py-3"
													>
														<span className="text-xs">Portfolio</span>
														<span className="text-[10px] opacity-70">Value tracking</span>
													</Button>
												</div>
											</FormControl>
											<FormMessage />
										</FormItem>
									)}
								/>

								{/* Domain Expiry Alerts */}
								<div className="space-y-4 rounded-lg border p-4">
									<h4 className="font-medium text-sm">Domain Expiry Alerts</h4>
									
									<FormField
										control={form.control}
										name="notify_on_expiry"
										render={({ field }) => (
											<FormItem className="flex items-center justify-between">
												<div className="space-y-0.5">
													<FormLabel>Notify before expiry</FormLabel>
													<p className="text-xs text-muted-foreground">Alert when domain is about to expire</p>
												</div>
												<FormControl>
													<Switch
														checked={field.value}
														onCheckedChange={field.onChange}
													/>
												</FormControl>
											</FormItem>
										)}
									/>

									{form.watch("notify_on_expiry") && (
										<FormField
											control={form.control}
											name="alert_days_before"
											render={({ field }) => (
												<FormItem className="pl-4 border-l-2">
													<FormLabel>Days before expiry</FormLabel>
													<FormControl>
														<Input type="number" min={1} max={365} className="w-32" {...field} value={field.value as string | number | undefined} />
													</FormControl>
													<FormMessage />
												</FormItem>
											)}
										/>
									)}
								</div>

								{/* SSL Alerts */}
								<div className="space-y-4 rounded-lg border p-4">
									<h4 className="font-medium text-sm">SSL Certificate Alerts</h4>
									
									<FormField
										control={form.control}
										name="notify_on_ssl_expiry"
										render={({ field }) => (
											<FormItem className="flex items-center justify-between">
												<div className="space-y-0.5">
													<FormLabel>Notify on SSL expiry</FormLabel>
													<p className="text-xs text-muted-foreground">Alert when SSL certificate expires</p>
												</div>
												<FormControl>
													<Switch
														checked={field.value}
														onCheckedChange={field.onChange}
													/>
												</FormControl>
											</FormItem>
										)}
									/>

									{form.watch("notify_on_ssl_expiry") && (
										<FormField
											control={form.control}
											name="ssl_alert_days"
											render={({ field }) => (
												<FormItem className="pl-4 border-l-2">
													<FormLabel>Days before SSL expiry</FormLabel>
													<FormControl>
														<Input type="number" min={1} max={90} className="w-32" {...field} value={field.value as string | number | undefined} />
													</FormControl>
													<FormMessage />
												</FormItem>
											)}
										/>
									)}
								</div>

								{/* Change Detection Alerts */}
								<div className="space-y-4 rounded-lg border p-4">
									<h4 className="font-medium text-sm">Change Detection</h4>
									
									<FormField
										control={form.control}
										name="notify_on_dns_change"
										render={({ field }) => (
											<FormItem className="flex items-center justify-between">
												<div className="space-y-0.5">
													<FormLabel>DNS changes</FormLabel>
													<p className="text-xs text-muted-foreground">Alert when DNS records change</p>
												</div>
												<FormControl>
													<Switch
														checked={field.value}
														onCheckedChange={field.onChange}
													/>
												</FormControl>
											</FormItem>
										)}
									/>

									<FormField
										control={form.control}
										name="notify_on_registrar_change"
										render={({ field }) => (
											<FormItem className="flex items-center justify-between">
												<div className="space-y-0.5">
													<FormLabel>Registrar changes</FormLabel>
													<p className="text-xs text-muted-foreground">Alert when registrar information changes</p>
												</div>
												<FormControl>
													<Switch
														checked={field.value}
														onCheckedChange={field.onChange}
													/>
												</FormControl>
											</FormItem>
										)}
									/>

									<FormField
										control={form.control}
										name="notify_on_value_change"
										render={({ field }) => (
											<FormItem className="flex items-center justify-between">
												<div className="space-y-0.5">
													<FormLabel>Value changes</FormLabel>
													<p className="text-xs text-muted-foreground">Alert when estimated value changes significantly</p>
												</div>
												<FormControl>
													<Switch
														checked={field.value}
														onCheckedChange={field.onChange}
													/>
												</FormControl>
											</FormItem>
										)}
									/>

									{form.watch("notify_on_value_change") && (
										<FormField
											control={form.control}
											name="value_change_threshold"
											render={({ field }) => (
												<FormItem className="pl-4 border-l-2">
													<FormLabel>Change threshold (%)</FormLabel>
													<FormControl>
														<Input type="number" min={1} max={100} className="w-32" {...field} value={field.value as string | number | undefined} />
													</FormControl>
													<FormMessage />
												</FormItem>
											)}
										/>
									)}
								</div>

								{/* Quiet Hours */}
								<div className="space-y-4 rounded-lg border p-4">
									<h4 className="font-medium text-sm">Quiet Hours</h4>
									
									<FormField
										control={form.control}
										name="quiet_hours_enabled"
										render={({ field }) => (
											<FormItem className="flex items-center justify-between">
												<div className="space-y-0.5">
													<FormLabel>Enable quiet hours</FormLabel>
													<p className="text-xs text-muted-foreground">Suppress notifications during specific hours</p>
												</div>
												<FormControl>
													<Switch
														checked={field.value}
														onCheckedChange={field.onChange}
													/>
												</FormControl>
											</FormItem>
										)}
									/>

									{form.watch("quiet_hours_enabled") && (
										<div className="grid grid-cols-2 gap-4 pl-4 border-l-2">
											<FormField
												control={form.control}
												name="quiet_hours_start"
												render={({ field }) => (
													<FormItem>
														<FormLabel>Start time</FormLabel>
														<FormControl>
															<Input type="time" {...field} value={field.value as string | number | undefined} />
														</FormControl>
													</FormItem>
												)}
											/>
											<FormField
												control={form.control}
												name="quiet_hours_end"
												render={({ field }) => (
													<FormItem>
														<FormLabel>End time</FormLabel>
														<FormControl>
															<Input type="time" {...field} value={field.value as string | number | undefined} />
														</FormControl>
													</FormItem>
												)}
											/>
										</div>
									)}
								</div>
							</TabsContent>
						</Tabs>

						<DialogFooter className="mt-6">
							<Button
								type="button"
								variant="outline"
								onClick={() => onOpenChange(false)}
							>
								Cancel
							</Button>
							<Button type="submit" disabled={isPending}>
								{isPending ? "Saving..." : isEdit ? "Update" : "Add Domain"}
							</Button>
						</DialogFooter>
					</form>
				</Form>
			</DialogContent>
		</Dialog>
	)
}
