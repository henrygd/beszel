import { $settings, pb } from '@/lib/stores'
import { useStore } from '@nanostores/react'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle, DialogTrigger } from './ui/dialog'
import { Button } from './ui/button'
import { SettingsIcon } from 'lucide-react'
import { AccordianItem, AccordianRoot, AccordionContent, AccordionTrigger } from './ui/accordian'
import { Switch } from './ui/switch'
import { useState } from 'react'
import { cn } from '@/lib/utils'
import { Input } from './ui/input'
import { toast } from './ui/use-toast'

export enum SettingsKeys {
	/** NTFY SETTINGS **/
	/** ntfy enabled */
	ntfy_enabled,
	/** server url */
	ntfy_url,
	/** username */
	ntfy_user,
	/** password */
	ntfy_pass,
	/** ntfy subject */
	ntfy_subject,
	/** ntfy_body */
	ntfy_body,

	/** SMTP SETTINGS **/
	/** smtp enabled */
	smtp_enabled
}

export default function SettingsButton() {
    const settings = useStore($settings)
    const [pendingChange, setPendingChange] = useState(false)

    // Filter settings by id with memo to given target setting
    const filteredSettings = (enumValue: number) => {
        const foundSetting =  settings.find((setting) => setting.enum === enumValue)
        return foundSetting ? foundSetting.value : '';
    }

    const ntfyLoginValidation = () => {
        const ntfyUrl = filteredSettings(SettingsKeys.ntfy_url)
        const ntfyUser = filteredSettings(SettingsKeys.ntfy_user)
        const ntfyPass = filteredSettings(SettingsKeys.ntfy_pass)
        if (!ntfyUrl) {
            return "Ntfy url is required"
        }
        if (!ntfyPass.startsWith('tk_') && (!ntfyUser || !ntfyPass)) {
            return "Ntfy username and password are required unless password is a token"
        }
        if (ntfyPass.startsWith('tk_') && !ntfyPass) {
            return "Token is required with no username"
        }
        return true
    }

    const testNtfy = async () => {
        const ntfyUrl = filteredSettings(SettingsKeys.ntfy_url)
        const ntfyUser = filteredSettings(SettingsKeys.ntfy_user)
        var ntfyPass = filteredSettings(SettingsKeys.ntfy_pass)
        const ntfySubject = "Beszel test subject"
        const ntfyBody = "Beszel test body"
        if (ntfyLoginValidation() !== true) {
            return
        }
        if (ntfyUser && ntfyPass) {
            ntfyPass = "Basic " + btoa(`${ntfyUser}:${ntfyPass}`)
        } else {
            ntfyPass = "Bearer " + ntfyPass
        }
        const response = await fetch(ntfyUrl, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                'Authorization': `${ntfyPass}`,
                'Title': ntfySubject
            },
            body: ntfyBody
        })
        if (response.status === 200) {
            toast({
                description: 'Ntfy test successful',
                duration: 5000
            })
        } else {
            toast({
                description: 'Ntfy test failed',
                duration: 5000
            })
        }
    }

    const insertOrUpdateSetting = async (key: number, value: object) => {
        if (pendingChange) {
            return
        }
        setPendingChange(true)
        const record = (await pb.collection('settings').getFullList()).filter((record) => record.enum === key)[0];
        if (!record) {
            try {
                const newRecord = pb.collection('settings').create(value);
            } catch (error: any) {
                toast({
                    description: 'Failed to create setting',
                    duration: 5000
                })
            } finally {
                setPendingChange(false)
            }
        } else {
            try {
                const updatedRecord = pb.collection('settings').update(record.id, value);
            } catch (error: any) {
                toast({
                    description: 'Failed to create setting',
                    duration: 5000
                })
            } finally {
                setPendingChange(false)
            }
        }
    }

    return (
        <Dialog>
            <DialogTrigger asChild>
                <Button variant={'ghost'} size="icon">
                    <SettingsIcon className="h-[1.2rem] w-[1.2rem]" />
                </Button>
            </DialogTrigger>
            <DialogContent className="max-h-full overflow-auto">
                <DialogHeader>
                    <DialogTitle className="mb-1">Settings</DialogTitle>
                    <DialogDescription>
                        <span>
                            Holds settings regarding notifications.
                        </span>
                    </DialogDescription>
                </DialogHeader>
                {/** Header for ntfy */}
                <div className="flex justify-between items-center mb-2">
                    <span className="font-medium">Notifications</span>
                </div>
                <AccordianRoot type="single" collapsible>
                    <AccordianItem value="item-1">
                        <AccordionTrigger>Email</AccordionTrigger>
                        <AccordionContent>
                            <div className="grid gap-3">
                                <div className="rounded-lg border">
                                    <label
                                        className={cn('flex flex-row items-center justify-between gap-4 cursor-pointer p-4')}
                                    >
                                        <div className="grid gap-1 select-none">
                                            <p className="font-semibold">Email enabled</p>
                                            <span className="block text-sm text-foreground opacity-80">To enable email notifications</span>
                                        </div>
                                        <Switch
                                        defaultChecked={filteredSettings(SettingsKeys.smtp_enabled)}
                                        onCheckedChange={async (active) => {
                                            insertOrUpdateSetting(SettingsKeys.smtp_enabled, {
                                                value: active,
                                                //user: pb.authStore.model?.id,
                                                enum: SettingsKeys.smtp_enabled.toString()
                                            })
                                        }} />
                                    </label>
                                </div>
                            </div>
                        </AccordionContent>
                    </AccordianItem>
                    <AccordianItem value="item-2">
                        <AccordionTrigger>Ntfy</AccordionTrigger>
                        <AccordionContent>
                            <div className="grid gap-3">
                                <div className="rounded-lg border">
                                    <label
                                        className={cn('flex flex-row items-center justify-between gap-4 cursor-pointer p-4')}
                                    >
                                        <div className="grid gap-1 select-none">
                                            <p className="font-semibold">Ntfy Enabled</p>
                                            <span className="block text-sm text-foreground opacity-80">To enable ntfy notifications</span>
                                        </div>
                                        <Switch
                                            defaultChecked={filteredSettings(SettingsKeys.ntfy_enabled)}
                                            onCheckedChange={async (active) => {
                                                insertOrUpdateSetting(SettingsKeys.ntfy_enabled, {
                                                    value: active,
                                                    //user: pb.authStore.model?.id,
                                                    enum: SettingsKeys.ntfy_enabled.toString()
                                                })
                                            }} />
                                    </label>
                                </div>
                            </div>
                            <div className="grid gap-3">
                                <div className="rounded-lg border">
                                    <label
                                        className={cn('flex flex-row items-center justify-between gap-4 cursor-pointer p-4')}
                                    >
                                        <div className="grid gap-1 select-none">
                                            <p className="font-semibold">Ntfy Url</p>
                                        </div>
                                        <Input
                                            defaultValue={filteredSettings(SettingsKeys.ntfy_url)}
                                            onChange={async (event) => {
                                                insertOrUpdateSetting(SettingsKeys.ntfy_url, {
                                                    value: event.target.value,
                                                    //user: pb.authStore.model?.id,
                                                    enum: SettingsKeys.ntfy_url.toString()
                                                })
                                            }} />
                                    </label>
                                </div>
                            </div>
                            <div className="grid gap-3">
                                <div className="rounded-lg border">
                                    <label
                                        className={cn('flex flex-row items-center justify-between gap-4 cursor-pointer p-4')}
                                    >
                                        <div className="grid gap-1 select-none">
                                            <p className="font-semibold">Ntfy Username</p>
                                        </div>
                                        <Input
                                            defaultValue={filteredSettings(SettingsKeys.ntfy_user)}
                                            onChange={async (event) => {
                                                insertOrUpdateSetting(SettingsKeys.ntfy_user, {
                                                    value: event.target.value,
                                                    //user: pb.authStore.model?.id,
                                                    enum: SettingsKeys.ntfy_user.toString()
                                                })
                                            }} />
                                    </label>
                                </div>
                            </div>
                            <div className="grid gap-3">
                                <div className="rounded-lg border">
                                    <label
                                        className={cn('flex flex-row items-center justify-between gap-4 cursor-pointer p-4')}
                                    >
                                        <div className="grid gap-1 select-none">
                                            <p className="font-semibold">Ntfy Password</p>
                                        </div>
                                        <Input
                                            defaultValue={filteredSettings(SettingsKeys.ntfy_pass)}
                                            type="password"
                                            onChange={async (event) => {
                                                insertOrUpdateSetting(SettingsKeys.ntfy_pass, {
                                                    value: event.target.value,
                                                    //user: pb.authStore.model?.id,
                                                    enum: SettingsKeys.ntfy_pass.toString()
                                                })
                                            }} />
                                    </label>
                                </div>
                            </div>
                            <div className="grid gap-3">
                                <div className={cn("rounded-lg flex flex-row items-center justify-between gap-4 cursor-pointer p-4 text-red-300")}>
                                    {ntfyLoginValidation() !== true ? ntfyLoginValidation() : <span></span>}
                                    <Button onClick={testNtfy} disabled={ntfyLoginValidation() !== true}>Test</Button>
                                </div>
                            </div>
                        </AccordionContent>
                    </AccordianItem>
                </AccordianRoot>
            </DialogContent>
        </Dialog>
    )
}

