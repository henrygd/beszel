import React from 'react';
import * as Accordion from '@radix-ui/react-accordion';
import { ChevronDownIcon } from 'lucide-react';

import { cn } from "@/lib/utils"
import './accordian.css';

const AccordianRoot = React.forwardRef<
    React.ElementRef<typeof Accordion.Root>,
    React.ComponentPropsWithoutRef<typeof Accordion.Root>
>(({ className, children, ...props }, forwardedRef) => (
    <Accordion.Root className={cn("AccordionRoot", className)} {...props} ref={forwardedRef}>
        {children}
    </Accordion.Root>
));

const AccordianItem = React.forwardRef<
    React.ElementRef<typeof Accordion.Item>,
    React.ComponentPropsWithoutRef<typeof Accordion.Item>
>(({ className, children, ...props }, forwardedRef) => (
    <Accordion.Item className={cn("AccordionItem", className)} {...props} ref={forwardedRef}>
        {children}
    </Accordion.Item>
));

const AccordionTrigger = React.forwardRef<
    React.ElementRef<typeof Accordion.Trigger>,
    React.ComponentPropsWithoutRef<typeof Accordion.Trigger>
>(({ className, children, ...props }, forwardedRef) => (
    <Accordion.Header className="AccordionHeader  font-semibold">
        <Accordion.Trigger
            className={cn(
                "AccordionTrigger",
                className
            )}
            {...props}
            ref={forwardedRef}
        >
            {children}
            <ChevronDownIcon className="AccordionChevron" aria-hidden />
        </Accordion.Trigger>
    </Accordion.Header>
));

const AccordionContent = React.forwardRef<
    React.ElementRef<typeof Accordion.Content>,
    React.ComponentPropsWithoutRef<typeof Accordion.Content>
>(({ className, children, ...props }, forwardedRef) => (
    <Accordion.Content
        className={cn(
            "AccordionContent text-sm text-foreground opacity-80",
            className
        )}
        {...props}
        ref={forwardedRef}
    >
        <div className="AccordionContentText">{children}</div>
    </Accordion.Content>
));

export { AccordionTrigger, AccordionContent, AccordianRoot, AccordianItem };