import { AlertRecord } from "@/types";
import { alertInfo } from "@/lib/utils";

export function groupAlerts(alerts: AlertRecord[]) {
  const groupMap = new Map<string, { name: string; value: number; min: number; alerts: AlertRecord[] }>();
  for (const alert of alerts) {
    const key = `${alert.name}|${alert.value}|${alert.min}`;
    if (!groupMap.has(key)) {
      groupMap.set(key, { name: alert.name, value: alert.value, min: alert.min, alerts: [] });
    }
    groupMap.get(key)!.alerts.push(alert);
  }
  return Array.from(groupMap.values());
}

export function getAlertIcon(name: string) {
  return alertInfo[name]?.icon;
}

export function getAlertLabel(name: string) {
  return alertInfo[name]?.name() ?? name;
} 