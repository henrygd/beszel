package alerts

import "testing"

func TestFormatSystemAlertBodyThresholdRelation(t *testing.T) {
	tests := []struct {
		name  string
		alert SystemAlertData
		want  string
	}{
		{
			name:  "cpu above threshold",
			alert: SystemAlertData{name: "CPU", descriptor: "CPU", val: 92.3, threshold: 80, triggered: true, unit: "%", min: 10},
			want:  "CPU averaged 92.30% for the previous 10 minutes (above 80% threshold).",
		},
		{
			name:  "cpu recovered below threshold",
			alert: SystemAlertData{name: "CPU", descriptor: "CPU", val: 71, threshold: 80, triggered: false, unit: "%", min: 10},
			want:  "CPU averaged 71.00% for the previous 10 minutes (below 80% threshold).",
		},
		{
			name:  "recovery at threshold",
			alert: SystemAlertData{name: "Memory", descriptor: "Memory", val: 80, threshold: 80, triggered: false, unit: "%", min: 5},
			want:  "Memory averaged 80.00% for the previous 5 minutes (at or below 80% threshold).",
		},
		{
			name:  "temperature above threshold",
			alert: SystemAlertData{name: "Temperature", descriptor: "Highest sensor CPU", val: 81.25, threshold: 80, triggered: true, unit: "°C", min: 5},
			want:  "Highest sensor CPU averaged 81.25°C for the previous 5 minutes (above 80°C threshold).",
		},
		{
			name:  "temperature recovered",
			alert: SystemAlertData{name: "Temperature", descriptor: "Highest sensor CPU", val: 79.5, threshold: 80, triggered: false, unit: "°C", min: 5},
			want:  "Highest sensor CPU averaged 79.50°C for the previous 5 minutes (below 80°C threshold).",
		},
		{
			name:  "disk usage keeps descriptor",
			alert: SystemAlertData{name: "Disk usage", descriptor: "Usage of /mnt/data", val: 95.5, threshold: 90, triggered: true, unit: "%", min: 1},
			want:  "Usage of /mnt/data averaged 95.50% for the previous 1 minute (above 90% threshold).",
		},
		{
			name:  "bandwidth unit and fractional threshold",
			alert: SystemAlertData{name: "Bandwidth", descriptor: "Bandwidth", val: 130.4, threshold: 112.5, triggered: true, unit: " MB/s", min: 3},
			want:  "Bandwidth averaged 130.40 MB/s for the previous 3 minutes (above 112.5 MB/s threshold).",
		},
		{
			name:  "unitless load average",
			alert: SystemAlertData{name: "5m Load", descriptor: "5m Load", val: 6.2, threshold: 4, triggered: true, unit: "", min: 5},
			want:  "5m Load averaged 6.20 for the previous 5 minutes (above 4 threshold).",
		},
		{
			name:  "battery low alert is below",
			alert: SystemAlertData{name: "Battery", descriptor: "Battery", val: 12, threshold: 20, triggered: true, unit: "%", min: 1},
			want:  "Battery averaged 12.00% for the previous 1 minute (below 20% threshold).",
		},
		{
			name:  "battery recovery is above",
			alert: SystemAlertData{name: "Battery", descriptor: "Battery", val: 35, threshold: 20, triggered: false, unit: "%", min: 1},
			want:  "Battery averaged 35.00% for the previous 1 minute (above 20% threshold).",
		},
		{
			name:  "battery recovery at threshold",
			alert: SystemAlertData{name: "Battery", descriptor: "Battery", val: 20, threshold: 20, triggered: false, unit: "%", min: 1},
			want:  "Battery averaged 20.00% for the previous 1 minute (at or above 20% threshold).",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			minutesLabel := "minute"
			if tt.alert.min > 1 {
				minutesLabel += "s"
			}
			if got := formatSystemAlertBody(tt.alert, minutesLabel); got != tt.want {
				t.Fatalf("formatSystemAlertBody() = %q, want %q", got, tt.want)
			}
		})
	}
}
