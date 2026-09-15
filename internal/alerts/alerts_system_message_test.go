package alerts

import "testing"

func TestFormatSystemAlertBodyTemperatureRelation(t *testing.T) {
	tests := []struct {
		name      string
		val       float64
		triggered bool
		want      string
	}{
		{name: "above threshold alert", val: 81.25, triggered: true, want: "Highest sensor CPU above 81.25°C for the previous 5 minutes."},
		{name: "below threshold recovery", val: 79.5, triggered: false, want: "Highest sensor CPU below 79.50°C for the previous 5 minutes."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			alert := SystemAlertData{
				name:       "Temperature",
				descriptor: "Highest sensor CPU",
				val:        tt.val,
				threshold:  80,
				triggered:  tt.triggered,
				unit:       "°C",
				min:        5,
			}
			if got := formatSystemAlertBody(alert, "minutes"); got != tt.want {
				t.Fatalf("formatSystemAlertBody() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatSystemAlertBodyOtherAlertUnchanged(t *testing.T) {
	alert := SystemAlertData{
		name:       "CPU",
		descriptor: "CPU",
		val:        91.5,
		threshold:  90,
		unit:       "%",
		min:        1,
	}
	want := "CPU averaged 91.50% for the previous 1 minute."
	if got := formatSystemAlertBody(alert, "minute"); got != want {
		t.Fatalf("formatSystemAlertBody() = %q, want %q", got, want)
	}
}
