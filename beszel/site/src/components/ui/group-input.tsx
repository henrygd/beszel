import { Badge } from "./badge"
import { Button } from "./button"
import { DropdownMenu, DropdownMenuContent, DropdownMenuTrigger } from "./dropdown-menu"
import { PlusIcon, XIcon } from "lucide-react"
import React from "react"
import { useDebounce } from "@/lib/useDebounce"
import { Command, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList } from "./command"

interface GroupInputProps {
  value: string
  groups: string[]
  onChange: (group: string) => void
  disabled?: boolean
}

export const GroupInput: React.FC<GroupInputProps> = ({ value, groups, onChange, disabled }) => {
  const [pending, setPending] = React.useState("")
  const [dropdownOpen, setDropdownOpen] = React.useState(false)
  const inputRef = React.useRef<HTMLInputElement>(null)

  React.useEffect(() => {
    if (!value) setPending("")
  }, [value])

  // Debounced filtering for better performance
  const debouncedPending = useDebounce(pending, 200)
  
  // Filter groups based on debounced pending input
  const filteredGroups = React.useMemo(() => {
    if (!debouncedPending) return groups
    return groups.filter(g => g.toLowerCase().includes(debouncedPending.toLowerCase()))
  }, [groups, debouncedPending])

  // Only show suggestions dropdown when the + button is clicked (DropdownMenu)
  // Remove the suggestions dropdown from the input area
  return (
    <div className="flex items-start gap-2">
      <div
        className={
          "bg-background min-h-10 flex flex-wrap items-center max-w-[30rem] rounded-md border px-3 py-2 text-sm placeholder:text-muted-foreground has-[:focus-visible]:outline-none ring-offset-background has-[:focus-visible]:ring-2 has-[:focus-visible]:ring-ring has-[:focus-visible]:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50 overflow-y-auto"
        }
        style={{ maxHeight: '10rem' }}
      >
        {value ? (
          <Badge className="flex-shrink-0">
            {value}
            <Button
              variant="ghost"
              size="icon"
              className="ms-2 h-3 w-3"
              onClick={() => onChange("")}
              disabled={disabled}
            >
              <XIcon className="w-3" />
            </Button>
          </Badge>
        ) : (
          <input
            ref={inputRef}
            type="text"
            className="flex-1 min-w-0 min-w-[100px] outline-none bg-background placeholder:text-muted-foreground max-w-full"
            placeholder="Type group..."
            value={pending}
            onChange={e => setPending(e.target.value)}
            onKeyDown={e => {
              if ((e.key === "Enter" || e.key === "Tab") && pending.trim()) {
                e.preventDefault()
                // If there's a matching suggestion, use it
                const match = filteredGroups.find(g => g.toLowerCase() === pending.trim().toLowerCase())
                if (match) {
                  onChange(match)
                } else {
                  onChange(pending.trim())
                }
                setPending("")
                setDropdownOpen(false)
              }
            }}
            disabled={disabled}
            autoComplete="off"
          />
        )}
      </div>
      <DropdownMenu open={dropdownOpen} onOpenChange={setDropdownOpen}>
        <DropdownMenuTrigger asChild>
          <Button variant="outline" size="icon" className="shrink-0 w-10 mt-1" disabled={disabled}>
            <PlusIcon className="h-4 w-4" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-64 p-0 max-h-80">
          <Command>
            <CommandInput placeholder="Search groups..." />
            <CommandList className="max-h-64">
              <CommandEmpty>No groups found.</CommandEmpty>
              {groups.length > 0 && (
                <CommandGroup heading="Existing Groups">
                  {groups.map((group) => (
                    <CommandItem
                      key={group}
                      onSelect={() => {
                        onChange(group)
                        setPending("")
                        setDropdownOpen(false)
                      }}
                    >
                      {group}
                    </CommandItem>
                  ))}
                </CommandGroup>
              )}
            </CommandList>
          </Command>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  )
} 