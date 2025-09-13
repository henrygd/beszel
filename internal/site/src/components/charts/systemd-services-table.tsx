import { SystemdService } from "@/types";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Trans } from "@lingui/react/macro";
import { memo, useMemo, useState } from "react";
import { Button } from "@/components/ui/button";
import { ChevronDownIcon, XIcon } from "lucide-react";
import { cn } from "@/lib/utils";
import { Input } from "@/components/ui/input";

interface SystemdServicesTableProps {
    services: SystemdService[];
}

const statusPriority: { [key: string]: number } = {
    failed: 1,
    activating: 2,
    active: 3,
    deactivating: 4,
    inactive: 5,
};

const getStatusColor = (status: string) => {
    switch (status) {
        case 'active':
            return 'text-green-500';
        case 'failed':
            return 'text-red-500';
        case 'activating':
        case 'reloading':
            return 'text-blue-500';
        case 'inactive':
        case 'deactivating':
            return 'text-gray-500';
        default:
            return '';
    }
};

const getStatusDotColor = (status: string) => {
    switch (status) {
        case 'active':
            return 'bg-green-500';
        case 'failed':
            return 'bg-red-500';
        case 'activating':
        case 'reloading':
            return 'bg-blue-500';
        case 'inactive':
        case 'deactivating':
            return 'bg-gray-500';
        default:
            return 'bg-gray-400';
    }
};


export default memo(function SystemdServicesTable({ services }: SystemdServicesTableProps) {
    const [isExpanded, setIsExpanded] = useState(false);
    const [filter, setFilter] = useState("");

    const sortedServices = useMemo(() => {
        return [...services].sort((a, b) => {
            const priorityA = statusPriority[a.status] || 99;
            const priorityB = statusPriority[b.status] || 99;
            if (priorityA !== priorityB) {
                return priorityA - priorityB;
            }
            return a.name.localeCompare(b.name);
        });
    }, [services]);

    const failedServices = useMemo(() => sortedServices.filter(s => s.status === 'failed'), [sortedServices]);
    const activeServicesCount = useMemo(() => services.filter(s => s.status === 'active').length, [services]);

    const filteredServices = useMemo(() => {
        if (!filter) {
            return sortedServices;
        }
        return sortedServices.filter(service => service.name.toLowerCase().includes(filter.toLowerCase()));
    }, [sortedServices, filter]);

    const servicesToShow = isExpanded ? filteredServices : failedServices;

    const summary = (
        <span className="text-sm text-muted-foreground ml-2">
            ({failedServices.length} <Trans>failed</Trans>, {activeServicesCount} <Trans>active</Trans>)
        </span>
    );

    return (
        <div>
            <div className="flex justify-between items-center mb-2">
                <h3 className="text-lg font-semibold">
                    <Trans>Systemd Services</Trans> {summary}
                </h3>
                {isExpanded && (
                    <div className="relative max-w-xs">
                        <Input
                            placeholder="Filter services..."
                            value={filter}
                            onChange={(e) => setFilter(e.target.value)}
                            className="ps-4 pe-8"
                        />
                        {filter && (
                            <Button
                                type="button"
                                variant="ghost"
                                size="icon"
                                aria-label="Clear"
                                className="absolute right-1 top-1/2 -translate-y-1/2 h-7 w-7 text-gray-500 hover:text-gray-900 dark:text-gray-400 dark:hover:text-gray-100"
                                onClick={() => setFilter("")}
                            >
                                <XIcon className="h-4 w-4" />
                            </Button>
                        )}
                    </div>
                )}
            </div>
            <div className="rounded-md border">
                <Table>
                    <TableHeader>
                        <TableRow>
                            <TableHead><Trans>Service</Trans></TableHead>
                            <TableHead><Trans>Status</Trans></TableHead>
                        </TableRow>
                    </TableHeader>
                    <TableBody>
                        {servicesToShow.map((service) => (
                            <TableRow key={service.name}>
                                <TableCell className="font-medium">{service.name}</TableCell>
                                <TableCell className={cn("flex items-center gap-2", getStatusColor(service.status))}>
                                    <span className={cn("h-2 w-2 rounded-full", getStatusDotColor(service.status))} />
                                    {service.status}
                                </TableCell>
                            </TableRow>
                        ))}
                    </TableBody>
                </Table>
            </div>
            {failedServices.length === 0 && !isExpanded && (
                <div className="text-center py-4 text-muted-foreground">
                    <Trans>No failed services.</Trans>
                </div>
            )}
            <div className="flex justify-center mt-2">
                <Button variant="ghost" onClick={() => setIsExpanded(!isExpanded)} className="flex items-center gap-1">
                    <span>
                        {isExpanded ? <Trans>Show less</Trans> : <Trans>Show all</Trans>} ({services.length})
                    </span>
                    <ChevronDownIcon className={cn("h-4 w-4 transition-transform", isExpanded && "rotate-180")} />
                </Button>
            </div>
        </div>
    );
})