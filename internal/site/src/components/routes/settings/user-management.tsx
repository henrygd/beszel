import { Trans, useLingui } from "@lingui/react/macro"
import { redirectPage } from "@nanostores/router"
import { PlusIcon, Trash2Icon, UserPenIcon } from "lucide-react"
import { memo, useEffect, useState } from "react"
import { $router } from "@/components/router"
import {
	AlertDialog,
	AlertDialogAction,
	AlertDialogCancel,
	AlertDialogContent,
	AlertDialogDescription,
	AlertDialogFooter,
	AlertDialogHeader,
	AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Button, buttonVariants } from "@/components/ui/button"
import {
	Dialog,
	DialogContent,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Separator } from "@/components/ui/separator"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { toast } from "@/components/ui/use-toast"
import { cn } from "@/lib/utils"
import { isAdmin, pb } from "@/lib/api"
import type { UserRecord } from "@/types"

type UserRole = "admin" | "user" | "readonly"

export default memo(function UserManagementPage() {
	if (!isAdmin()) {
		redirectPage($router, "settings", { name: "general" })
	}

	const { t } = useLingui()

	const roleLabel: Record<UserRole, string> = {
		admin: t`Admin`,
		user: t`User`,
		readonly: t`Read Only`,
	}
	const [users, setUsers] = useState<UserRecord[]>([])
	const [dialogOpen, setDialogOpen] = useState(false)
	const [editUser, setEditUser] = useState<UserRecord | null>(null)
	const [deleteUser, setDeleteUser] = useState<UserRecord | null>(null)
	const [isLoading, setIsLoading] = useState(false)

	const currentUserId = pb.authStore.record?.id

	useEffect(() => {
		pb.collection("users")
			.getFullList<UserRecord>({ sort: "created" })
			.then(setUsers)
	}, [])

	function openCreate() {
		setEditUser(null)
		setDialogOpen(true)
	}

	function openEdit(user: UserRecord) {
		setEditUser(user)
		setDialogOpen(true)
	}

	async function handleDelete() {
		if (!deleteUser) return
		try {
			await pb.collection("users").delete(deleteUser.id)
			setUsers((prev) => prev.filter((u) => u.id !== deleteUser.id))
			toast({ title: t`User deleted` })
		} catch {
			toast({ title: t`Failed to delete user`, variant: "destructive" })
		} finally {
			setDeleteUser(null)
		}
	}

	async function handleSubmit(e: React.FormEvent<HTMLFormElement>) {
		e.preventDefault()
		setIsLoading(true)
		const form = e.target as HTMLFormElement
		const data = Object.fromEntries(new FormData(form)) as Record<string, string>
		try {
			if (editUser) {
				const payload: Record<string, string> = { role: data.role }
				if (data.password) {
					payload.password = data.password
					payload.passwordConfirm = data.password
				}
				const updated = await pb.collection("users").update<UserRecord>(editUser.id, payload)
				setUsers((prev) => prev.map((u) => (u.id === updated.id ? updated : u)))
				toast({ title: t`User updated` })
			} else {
				const created = await pb.collection("users").create<UserRecord>({
					email: data.email,
					password: data.password,
					passwordConfirm: data.password,
					role: data.role,
				})
				setUsers((prev) => [...prev, created])
				toast({ title: t`User created` })
			}
			setDialogOpen(false)
		} catch (err: any) {
			toast({
				title: editUser ? t`Failed to update user` : t`Failed to create user`,
				description: err?.message,
				variant: "destructive",
			})
		} finally {
			setIsLoading(false)
		}
	}

	return (
		<div>
			<div className="flex items-start justify-between gap-4">
				<div>
					<h3 className="text-xl font-medium mb-2">
						<Trans>Users</Trans>
					</h3>
					<p className="text-sm text-muted-foreground leading-relaxed">
						<Trans>Manage user accounts and roles.</Trans>
					</p>
				</div>
				<Button size="sm" className="flex items-center gap-1.5 shrink-0 mt-0.5" onClick={openCreate}>
					<PlusIcon className="h-4 w-4" />
					<Trans>Add User</Trans>
				</Button>
			</div>
			<Separator className="my-4" />
			<div className="rounded-md border overflow-hidden w-full">
				<Table>
					<TableHeader>
						<tr className="border-border/50">
							<TableHead>
								<Trans>Email</Trans>
							</TableHead>
							<TableHead>
								<Trans>Role</Trans>
							</TableHead>
							<TableHead>
								<Trans>Created</Trans>
							</TableHead>
							<TableHead className="w-0">
								<span className="sr-only">
									<Trans>Actions</Trans>
								</span>
							</TableHead>
						</tr>
					</TableHeader>
					<TableBody>
						{users.map((user) => (
							<TableRow key={user.id}>
								<TableCell className="font-medium">{user.email}</TableCell>
								<TableCell>{roleLabel[user.role as UserRole] ?? user.role}</TableCell>
								<TableCell className="text-muted-foreground text-sm">
									{new Date(user.created).toLocaleDateString()}
								</TableCell>
								<TableCell>
									<div className="flex items-center gap-1 justify-end">
										<Button variant="ghost" size="icon" className="h-7 w-7" onClick={() => openEdit(user)}>
											<UserPenIcon className="h-4 w-4" />
										</Button>
										<Button
											variant="ghost"
											size="icon"
											className="h-7 w-7 text-red-500 hover:text-red-500"
											disabled={user.id === currentUserId}
											onClick={() => setDeleteUser(user)}
										>
											<Trash2Icon className="h-4 w-4" />
										</Button>
									</div>
								</TableCell>
							</TableRow>
						))}
					</TableBody>
				</Table>
			</div>

			<Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
				<DialogContent className="sm:max-w-md">
					<DialogHeader>
						<DialogTitle>{editUser ? <Trans>Edit User</Trans> : <Trans>Add User</Trans>}</DialogTitle>
					</DialogHeader>
					<form onSubmit={handleSubmit} className="grid gap-4 py-2">
						{!editUser && (
							<div className="grid gap-2">
								<Label htmlFor="email">
									<Trans>Email</Trans>
								</Label>
								<Input id="email" name="email" type="email" required autoComplete="off" />
							</div>
						)}
						{editUser && (
							<div className="grid gap-2">
								<Label>
									<Trans>Email</Trans>
								</Label>
								<p className="text-sm text-muted-foreground py-1">{editUser.email}</p>
							</div>
						)}
						<div className="grid gap-2">
							<Label htmlFor="role">
								<Trans>Role</Trans>
							</Label>
							<Select name="role" defaultValue={editUser?.role ?? "user"}>
								<SelectTrigger id="role">
									<SelectValue />
								</SelectTrigger>
								<SelectContent>
									<SelectItem value="admin"><Trans>Admin</Trans></SelectItem>
									<SelectItem value="user"><Trans>User</Trans></SelectItem>
									<SelectItem value="readonly"><Trans>Read Only</Trans></SelectItem>
								</SelectContent>
							</Select>
						</div>
						<div className="grid gap-2">
							<Label htmlFor="password">
								{editUser ? <Trans>New Password (leave blank to keep)</Trans> : <Trans>Password</Trans>}
							</Label>
							<Input
								id="password"
								name="password"
								type="password"
								required={!editUser}
								autoComplete="new-password"
								minLength={8}
							/>
						</div>
						<DialogFooter className="mt-2">
							<Button type="submit" disabled={isLoading}>
								{editUser ? <Trans>Save</Trans> : <Trans>Create</Trans>}
							</Button>
						</DialogFooter>
					</form>
				</DialogContent>
			</Dialog>

			<AlertDialog open={!!deleteUser} onOpenChange={(open) => !open && setDeleteUser(null)}>
				<AlertDialogContent>
					<AlertDialogHeader>
						<AlertDialogTitle>
							<Trans>Are you sure you want to delete {deleteUser?.email}?</Trans>
						</AlertDialogTitle>
						<AlertDialogDescription>
							<Trans>This action cannot be undone.</Trans>
						</AlertDialogDescription>
					</AlertDialogHeader>
					<AlertDialogFooter>
						<AlertDialogCancel>
							<Trans>Cancel</Trans>
						</AlertDialogCancel>
						<AlertDialogAction className={cn(buttonVariants({ variant: "destructive" }))} onClick={handleDelete}>
							<Trans>Delete</Trans>
						</AlertDialogAction>
					</AlertDialogFooter>
				</AlertDialogContent>
			</AlertDialog>
		</div>
	)
})
