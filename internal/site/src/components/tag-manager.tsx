import { useState, useMemo } from "react"
import { useStore } from "@nanostores/react"
import { $systems } from "@/lib/stores"
import { refresh as refreshSystems } from "@/lib/systemsManager"
import {
  bulkRenameTag,
  bulkDeleteTag,
  getSystemsWithTag,
  type BulkRenameResult
} from "@/lib/bulkOperations"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Card, CardHeader, CardTitle, CardContent } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import { 
  Dialog, 
  DialogContent, 
  DialogDescription, 
  DialogFooter, 
  DialogHeader, 
  DialogTitle, 
  DialogTrigger 
} from "@/components/ui/dialog"
import { 
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog"
import { Separator } from "@/components/ui/separator"
import { Label } from "@/components/ui/label"
import {
  Edit2Icon,
  TrashIcon,
  Loader2,
  InfoIcon,
  TagIcon
} from "lucide-react"
import { toast } from "@/components/ui/use-toast"

interface RenameDialogProps {
  name: string
  affectedCount: number
  onRename: (oldName: string, newName: string) => Promise<BulkRenameResult>
  children: React.ReactNode
}

function RenameDialog({ name, affectedCount, onRename, children }: RenameDialogProps) {
  const [open, setOpen] = useState(false)
  const [newName, setNewName] = useState(name)
  const [loading, setLoading] = useState(false)

  const handleRename = async () => {
    if (!newName.trim() || newName === name) return

    setLoading(true)
    try {
      const result = await onRename(name, newName.trim())
      
      if (result.success) {
        toast({
          title: 'Tag renamed successfully',
          description: `Updated ${result.affectedSystems} systems`,
        })
        await refreshSystems()
        setOpen(false)
      } else {
        toast({
          title: "Rename failed",
          description: result.errors?.join(', ') || 'Unknown error occurred',
          variant: "destructive"
        })
      }
    } catch (error) {
      toast({
        title: "Rename failed", 
        description: `Error: ${error}`,
        variant: "destructive"
      })
    } finally {
      setLoading(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>{children}</DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Rename Tag</DialogTitle>
          <DialogDescription>
            This will rename "{name}" across {affectedCount} system{affectedCount !== 1 ? 's' : ''}.
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-4 py-4">
          <div className="grid gap-2">
            <Label htmlFor="newName">New tag name</Label>
            <Input
              id="newName"
              value={newName}
              onChange={(e) => setNewName(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && handleRename()}
            />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => setOpen(false)} disabled={loading}>
            Cancel
          </Button>
          <Button 
            onClick={handleRename}
            disabled={loading || !newName.trim() || newName === name}
          >
            {loading ? (
              <>
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                Renaming...
              </>
            ) : (
              'Rename Tag'
            )}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

interface DeleteDialogProps {
  name: string
  affectedCount: number
  onDelete: (name: string) => Promise<BulkRenameResult>
  children: React.ReactNode
}

function DeleteDialog({ name, affectedCount, onDelete, children }: DeleteDialogProps) {
  const [open, setOpen] = useState(false)
  const [loading, setLoading] = useState(false)

  const handleDelete = async () => {
    setLoading(true)
    try {
      const result = await onDelete(name)
      
      if (result.success) {
        toast({
          title: 'Tag deleted successfully',
          description: `Removed from ${result.affectedSystems} systems`,
        })
        await refreshSystems()
        setOpen(false)
      } else {
        toast({
          title: "Delete failed",
          description: result.errors?.join(', ') || 'Unknown error occurred',
          variant: "destructive"
        })
      }
    } catch (error) {
      toast({
        title: "Delete failed",
        description: `Error: ${error}`,
        variant: "destructive"
      })
    } finally {
      setLoading(false)
    }
  }

  return (
    <AlertDialog open={open} onOpenChange={setOpen}>
      <AlertDialogTrigger asChild>{children}</AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Delete Tag</AlertDialogTitle>
          <AlertDialogDescription>
            This will remove "{name}" from {affectedCount} system{affectedCount !== 1 ? 's' : ''}.
            This action cannot be undone.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={loading}>Cancel</AlertDialogCancel>
          <AlertDialogAction
            onClick={handleDelete}
            disabled={loading}
            className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
          >
            {loading ? (
              <>
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                Deleting...
              </>
            ) : (
              'Delete Tag'
            )}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}

export default function TagManager() {
  const systems = useStore($systems)

  // Get all unique tags with their usage counts
  const tagStats = useMemo(() => {
    const tagCounts = new Map<string, number>()
    systems.forEach(system => {
      system.tags?.forEach(tag => {
        tagCounts.set(tag, (tagCounts.get(tag) || 0) + 1)
      })
    })
    return Array.from(tagCounts.entries())
      .map(([name, count]) => ({ name, count }))
      .sort((a, b) => b.count - a.count)
  }, [systems])

  const handleTagRename = async (oldTag: string, newTag: string) => {
    return await bulkRenameTag(oldTag, newTag, systems)
  }

  const handleTagDelete = async (tag: string) => {
    return await bulkDeleteTag(tag, systems)
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-2">
        <InfoIcon className="h-5 w-5 text-muted-foreground" />
        <p className="text-sm text-muted-foreground">
          Manage your tags. Rename or delete them across all systems at once.
        </p>
      </div>

      {/* Tags Section */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <TagIcon className="h-5 w-5" />
            Tags ({tagStats.length})
          </CardTitle>
        </CardHeader>
        <CardContent>
          {tagStats.length === 0 ? (
            <p className="text-muted-foreground text-center py-8">
              No tags found. Add tags to your systems to manage them here.
            </p>
          ) : (
            <div className="space-y-3">
              {tagStats.map(({ name, count }) => (
                <div key={name} className="flex items-center justify-between p-3 border rounded-lg">
                  <div className="flex items-center gap-3">
                    <Badge variant="secondary">{name}</Badge>
                    <span className="text-sm text-muted-foreground">
                      Used by {count} system{count !== 1 ? 's' : ''}
                    </span>
                  </div>
                  <div className="flex items-center gap-2">
                    <RenameDialog
                      name={name}
                      affectedCount={count}
                      onRename={handleTagRename}
                    >
                      <Button variant="ghost" size="sm">
                        <Edit2Icon className="h-4 w-4" />
                      </Button>
                    </RenameDialog>
                    <DeleteDialog
                      name={name}
                      affectedCount={count}
                      onDelete={handleTagDelete}
                    >
                      <Button variant="ghost" size="sm" className="text-destructive hover:text-destructive">
                        <TrashIcon className="h-4 w-4" />
                      </Button>
                    </DeleteDialog>
                  </div>
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  )
}