import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import React from "react";

export default function AlertSheet({ open, onClose, title, children }: {
  open: boolean;
  onClose: () => void;
  title: string;
  children: React.ReactNode;
}) {
  return (
    <Sheet open={open} onOpenChange={onClose}>
      <SheetContent side="right" className="flex flex-col h-full w-[55em] max-w-3xl">
        <SheetHeader>
          <SheetTitle>{title}</SheetTitle>
        </SheetHeader>
        {children}
      </SheetContent>
    </Sheet>
  );
} 