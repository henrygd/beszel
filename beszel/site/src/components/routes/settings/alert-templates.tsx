import { t } from "@lingui/core/macro"
import { Trans } from "@lingui/react/macro"
import { useState, useEffect } from "react"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { toast } from "@/components/ui/use-toast"
import { Badge } from "@/components/ui/badge"
import { pb } from "@/lib/api"
import { AlertTemplateRecord } from "@/types"
import { EditIcon, RotateCcwIcon, EyeIcon, SaveIcon, XIcon } from "lucide-react"
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Collapsible } from "@/components/ui/collapsible"
import { alertInfo } from "@/lib/alerts"

const alertTypes = Object.keys(alertInfo) as (keyof typeof alertInfo)[]

interface TemplateData {
	title_template: string
	message_template: string
}

const defaultTemplates: Record<string, TemplateData> = {
	Status: {
		title_template: "Connection to {{systemName}} is {{status}} {{emoji}}",
		message_template: "Connection to {{systemName}} is {{status}}",
	},
	CPU: {
		title_template: "{{systemName}} {{alertName}} {{thresholdStatus}} threshold",
		message_template: "{{descriptor}} averaged {{value}}{{unit}} for the previous {{minutes}} {{minutesLabel}}.",
	},
	Memory: {
		title_template: "{{systemName}} {{alertName}} {{thresholdStatus}} threshold",
		message_template: "{{descriptor}} averaged {{value}}{{unit}} for the previous {{minutes}} {{minutesLabel}}.",
	},
	Disk: {
		title_template: "{{systemName}} {{alertName}} {{thresholdStatus}} threshold",
		message_template: "{{descriptor}} averaged {{value}}{{unit}} for the previous {{minutes}} {{minutesLabel}}.",
	},
	Temperature: {
		title_template: "{{systemName}} {{alertName}} {{thresholdStatus}} threshold",
		message_template: "{{descriptor}} averaged {{value}}{{unit}} for the previous {{minutes}} {{minutesLabel}}.",
	},
	Bandwidth: {
		title_template: "{{systemName}} {{alertName}} {{thresholdStatus}} threshold",
		message_template: "{{descriptor}} averaged {{value}}{{unit}} for the previous {{minutes}} {{minutesLabel}}.",
	},
	BandwidthUp: {
		title_template: "{{systemName}} {{alertName}} {{thresholdStatus}} threshold",
		message_template: "{{descriptor}} averaged {{value}}{{unit}} for the previous {{minutes}} {{minutesLabel}}.",
	},
	BandwidthDown: {
		title_template: "{{systemName}} {{alertName}} {{thresholdStatus}} threshold",
		message_template: "{{descriptor}} averaged {{value}}{{unit}} for the previous {{minutes}} {{minutesLabel}}.",
	},
	LoadAvg1: {
		title_template: "{{systemName}} {{alertName}} {{thresholdStatus}} threshold",
		message_template: "{{descriptor}} averaged {{value}}{{unit}} for the previous {{minutes}} {{minutesLabel}}.",
	},
	LoadAvg5: {
		title_template: "{{systemName}} {{alertName}} {{thresholdStatus}} threshold",
		message_template: "{{descriptor}} averaged {{value}}{{unit}} for the previous {{minutes}} {{minutesLabel}}.",
	},
	LoadAvg15: {
		title_template: "{{systemName}} {{alertName}} {{thresholdStatus}} threshold",
		message_template: "{{descriptor}} averaged {{value}}{{unit}} for the previous {{minutes}} {{minutesLabel}}.",
	},
	Swap: {
		title_template: "{{systemName}} {{alertName}} {{thresholdStatus}} threshold",
		message_template: "{{descriptor}} averaged {{value}}{{unit}} for the previous {{minutes}} {{minutesLabel}}.",
	},
}

const templateVariables = [
	{ name: "systemName", description: "Name of the system" },
	{ name: "alertName", description: "Name of the alert type" },
	{ name: "alertType", description: "Raw alert type (CPU, Disk, etc.)" },
	{ name: "thresholdStatus", description: "'above' or 'below'" },
	{ name: "status", description: "For status alerts: 'up' or 'down'" },
	{ name: "emoji", description: "Status emoji (✅ or 🔴)" },
	{ name: "value", description: "Alert value (85.50, 92.30, etc.)" },
	{ name: "unit", description: "Alert unit (%, °C, Mbps, etc.)" },
	{ name: "threshold", description: "Configured threshold value" },
	{ name: "minutes", description: "Duration in minutes" },
	{ name: "minutesLabel", description: "'minute' or 'minutes'" },
	{ name: "descriptor", description: "Alert descriptor (Usage of root, CPU, etc.)" },
	{ name: "filesystem", description: "Filesystem name (root, /var, etc.) - Disk alerts only" },
]

export default function AlertTemplatesSettings() {
	const [templates, setTemplates] = useState<Record<string, AlertTemplateRecord>>({})
	const [loading, setLoading] = useState(true)
	const [editingType, setEditingType] = useState<string | null>(null)
	const [showPreview, setShowPreview] = useState(false)
	const [previewData, setPreviewData] = useState<any>(null)

	const [editData, setEditData] = useState<TemplateData>({
		title_template: "",
		message_template: "",
	})

	useEffect(() => {
		loadTemplates()
	}, [])

	const loadTemplates = async () => {
		try {
			const records = await pb.collection("alert_templates").getFullList<AlertTemplateRecord>()
			const templateMap: Record<string, AlertTemplateRecord> = {}
			records.forEach(template => {
				templateMap[template.alert_type] = template
			})
			setTemplates(templateMap)
		} catch (error) {
			toast({
				title: t`Failed to load templates`,
				description: t`Check logs for more details.`,
				variant: "destructive",
			})
		} finally {
			setLoading(false)
		}
	}

	const getEffectiveTemplate = (alertType: string): TemplateData => {
		// Return custom template if exists, otherwise return default
		const customTemplate = templates[alertType]
		if (customTemplate) {
			return {
				title_template: customTemplate.title_template,
				message_template: customTemplate.message_template,
			}
		}
		return defaultTemplates[alertType] || defaultTemplates.CPU
	}

	const startEditing = (alertType: string) => {
		const template = getEffectiveTemplate(alertType)
		setEditData(template)
		setEditingType(alertType)
	}

	const cancelEditing = () => {
		setEditingType(null)
		setEditData({ title_template: "", message_template: "" })
	}

	const saveTemplate = async (alertType: string) => {
		try {
			const existingTemplate = templates[alertType]
			
			if (existingTemplate) {
				// Update existing template
				await pb.collection("alert_templates").update(existingTemplate.id, {
					title_template: editData.title_template,
					message_template: editData.message_template,
				})
			} else {
				// Create new template
				await pb.collection("alert_templates").create({
					name: `${alertInfo[alertType as keyof typeof alertInfo].name()} Template`,
					alert_type: alertType,
					title_template: editData.title_template,
					message_template: editData.message_template,
					user: pb.authStore.record?.id,
				})
			}
			
			toast({
				title: t`Template saved`,
				description: t`Alert template has been saved successfully.`,
			})
			
			setEditingType(null)
			loadTemplates()
		} catch (error) {
			toast({
				title: t`Failed to save template`,
				description: t`Check logs for more details.`,
				variant: "destructive",
			})
		}
	}

	const resetToDefault = async (alertType: string) => {
		if (!confirm(t`Reset this template to system default?`)) return

		try {
			const existingTemplate = templates[alertType]
			if (existingTemplate) {
				// Delete custom template to fall back to default
				await pb.collection("alert_templates").delete(existingTemplate.id)
				toast({
					title: t`Template reset`,
					description: t`Alert template has been reset to system default.`,
				})
				loadTemplates()
			}
		} catch (error) {
			toast({
				title: t`Failed to reset template`,
				description: t`Check logs for more details.`,
				variant: "destructive",
			})
		}
	}

	const previewTemplate = (alertType: string, templateData?: TemplateData) => {
		const template = templateData || getEffectiveTemplate(alertType)
		
		// Mock data for preview
		const mockData = {
			systemName: "server1",
			alertName: alertInfo[alertType as keyof typeof alertInfo]?.name() || alertType,
			alertType: alertType,
			thresholdStatus: "above",
			status: alertType === "Status" ? "down" : undefined,
			emoji: alertType === "Status" ? "🔴" : undefined,
			value: "85.50",
			unit: alertType === "Temperature" ? "°C" : alertType.includes("Bandwidth") ? " Mbps" : "%",
			threshold: "80.00",
			minutes: "5",
			minutesLabel: "minutes",
			descriptor: alertType === "Disk" ? "Usage of root" : alertType,
			filesystem: alertType === "Disk" ? "root" : undefined,
		}

		// Simple template variable replacement
		let title = template.title_template
		let message = template.message_template

		Object.entries(mockData).forEach(([key, value]) => {
			if (value !== undefined) {
				title = title.replace(new RegExp(`{{${key}}}`, 'g'), value)
				message = message.replace(new RegExp(`{{${key}}}`, 'g'), value)
			}
		})

		setPreviewData({ title, message, mockData })
		setShowPreview(true)
	}

	if (loading) {
		return <div>Loading...</div>
	}

	return (
		<div className="space-y-6">
			<div>
				<h3 className="text-lg font-medium">
					<Trans>Alert Templates</Trans>
				</h3>
				<p className="text-sm text-muted-foreground">
					<Trans>Customize notification templates for different alert types. Each alert type has one template that can be customized or reset to default.</Trans>
				</p>
			</div>

			<div className="grid gap-4">
				{alertTypes.map((alertType) => {
					const hasCustomTemplate = !!templates[alertType]
					const isEditing = editingType === alertType
					const effectiveTemplate = getEffectiveTemplate(alertType)

					return (
						<Collapsible 
							key={alertType}
							title={alertInfo[alertType].name()}
							description={alertInfo[alertType].desc()}
							defaultOpen={isEditing}
							icon={
								<div className="flex items-center gap-2">
									{!hasCustomTemplate && (
										<Badge variant="outline" className="text-xs">
											<Trans>System Default</Trans>
										</Badge>
									)}
									{hasCustomTemplate && (
										<Badge variant="default" className="text-xs">
											<Trans>Custom</Trans>
										</Badge>
									)}
								</div>
							}
						>
								{isEditing ? (
									<div className="space-y-4">
										<div>
											<Label htmlFor={`title-${alertType}`}>
												<Trans>Title Template</Trans>
											</Label>
											<Input
												id={`title-${alertType}`}
												value={editData.title_template}
												onChange={(e) => setEditData({ ...editData, title_template: e.target.value })}
												placeholder="{{systemName}} {{alertName}} {{thresholdStatus}} threshold"
											/>
										</div>

										<div>
											<Label htmlFor={`message-${alertType}`}>
												<Trans>Message Template</Trans>
											</Label>
											<Textarea
												id={`message-${alertType}`}
												value={editData.message_template}
												onChange={(e) => setEditData({ ...editData, message_template: e.target.value })}
												placeholder="{{descriptor}} averaged {{value}}{{unit}} for the previous {{minutes}} {{minutesLabel}}."
												rows={3}
											/>
										</div>

										<div className="flex justify-between">
											<Button
												variant="outline"
												onClick={() => previewTemplate(alertType, editData)}
												disabled={!editData.title_template || !editData.message_template}
											>
												<EyeIcon className="h-4 w-4 mr-2" />
												<Trans>Preview</Trans>
											</Button>
											<div className="flex gap-2">
												<Button variant="outline" onClick={cancelEditing}>
													<XIcon className="h-4 w-4 mr-2" />
													<Trans>Cancel</Trans>
												</Button>
												<Button onClick={() => saveTemplate(alertType)}>
													<SaveIcon className="h-4 w-4 mr-2" />
													<Trans>Save</Trans>
												</Button>
											</div>
										</div>
									</div>
								) : (
									<div className="space-y-3">
										<div className="text-sm">
											<div className="font-medium">Title:</div>
											<div className="text-muted-foreground bg-muted p-2 rounded text-xs font-mono">
												{effectiveTemplate.title_template}
											</div>
										</div>
										<div className="text-sm">
											<div className="font-medium">Message:</div>
											<div className="text-muted-foreground bg-muted p-2 rounded text-xs font-mono">
												{effectiveTemplate.message_template}
											</div>
										</div>
										<div className="flex gap-2">
											<Button
												variant="outline"
												size="sm"
												onClick={() => previewTemplate(alertType)}
											>
												<EyeIcon className="h-4 w-4 mr-1" />
												<Trans>Preview</Trans>
											</Button>
											<Button
												variant="outline"
												size="sm"
												onClick={() => startEditing(alertType)}
											>
												<EditIcon className="h-4 w-4 mr-1" />
												<Trans>Edit</Trans>
											</Button>
											{hasCustomTemplate && (
												<Button
													variant="outline"
													size="sm"
													onClick={() => resetToDefault(alertType)}
												>
													<RotateCcwIcon className="h-4 w-4 mr-1" />
													<Trans>Reset</Trans>
												</Button>
											)}
										</div>
									</div>
								)}
						</Collapsible>
					)
				})}
			</div>

			{/* Template Variables Help */}
			<Card>
				<CardHeader>
					<CardTitle className="text-base">
						<Trans>Available Template Variables</Trans>
					</CardTitle>
				</CardHeader>
				<CardContent>
					<div className="grid grid-cols-1 md:grid-cols-2 gap-4 text-sm">
						{templateVariables.map((variable) => (
							<div key={variable.name} className="flex gap-3">
								<code className="bg-muted px-2 py-1 rounded text-xs">
									{"{{" + variable.name + "}}"}
								</code>
								<span className="text-muted-foreground">{variable.description}</span>
							</div>
						))}
					</div>
				</CardContent>
			</Card>

			{/* Preview Dialog */}
			<Dialog open={showPreview} onOpenChange={setShowPreview}>
				<DialogContent>
					<DialogHeader>
						<DialogTitle><Trans>Template Preview</Trans></DialogTitle>
					</DialogHeader>
					{previewData && (
						<div className="space-y-4">
							<div>
								<Label><Trans>Title</Trans></Label>
								<div className="p-3 bg-muted rounded">{previewData.title}</div>
							</div>
							<div>
								<Label><Trans>Message</Trans></Label>
								<div className="p-3 bg-muted rounded">{previewData.message}</div>
							</div>
							<div>
								<Label><Trans>Sample Data Used</Trans></Label>
								<pre className="text-xs bg-muted p-2 rounded overflow-auto">
									{JSON.stringify(previewData.mockData, null, 2)}
								</pre>
							</div>
						</div>
					)}
				</DialogContent>
			</Dialog>
		</div>
	)
}