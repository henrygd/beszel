import { DropdownMenu, DropdownMenuTrigger, DropdownMenuContent, DropdownMenuCheckboxItem } from "@/components/ui/dropdown-menu";
import { Button } from "@/components/ui/button";
import React from "react";

export default function SystemsDropdown({
    options,
    value,
    onChange,
}: {
    options: { label: string; value: string }[];
    value: string[];
    onChange: (value: string[]) => void;
}) {
    return (
        <DropdownMenu>
            <DropdownMenuTrigger asChild>
                <Button variant="outline" className="w-full justify-between">
                    {value.length === 0
                        ? "Select systems..."
                        : options
                            .filter((o) => value.includes(o.value))
                            .map((o) => o.label)
                            .join(", ")}
                </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent className="w-64 max-h-60 overflow-auto">
                {options.map((option) => (
                    <DropdownMenuCheckboxItem
                        key={option.value}
                        checked={value.includes(option.value)}
                        onCheckedChange={(checked) => {
                            if (checked) {
                                onChange([...value, option.value]);
                            } else {
                                onChange(value.filter((id) => id !== option.value));
                            }
                        }}
                    >
                        {option.label}
                    </DropdownMenuCheckboxItem>
                ))}
            </DropdownMenuContent>
        </DropdownMenu>
    );
} 