package incidents

import (
	"net/http/httptest"
	"testing"
	"time"

	"ganigeorgiev.com/pocketbase/pocketbase/core"
	"ganigeorgiev.com/pocketbase/pocketbase/tools/router"
)

func TestDateInRange(t *testing.T) {
	from := mustDate(t, "2026-04-01")
	to := mustDate(t, "2026-04-30")

	tests := []struct {
		name string
		date string
		want bool
	}{
		{name: "before range", date: "2026-03-31", want: false},
		{name: "start boundary", date: "2026-04-01", want: true},
		{name: "inside range", date: "2026-04-15", want: true},
		{name: "end boundary", date: "2026-04-30", want: true},
		{name: "after range", date: "2026-05-01", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dateInRange(mustDate(t, tt.date), &from, &to)
			if got != tt.want {
				t.Fatalf("dateInRange(%s) = %v, want %v", tt.date, got, tt.want)
			}
		})
	}
}

func TestCalendarRangeParsesDateBounds(t *testing.T) {
	request := httptest.NewRequest("GET", "/api/beszel/incidents/calendar?from=2026-04-01&to=2026-04-30", nil)
	from, to := calendarRange(&core.RequestEvent{Event: router.Event{Request: request}})

	if from == nil || from.Format("2006-01-02") != "2026-04-01" {
		t.Fatalf("unexpected from date: %v", from)
	}
	if to == nil || !dateInRange(mustDate(t, "2026-04-30").Add(23*time.Hour), from, to) {
		t.Fatalf("expected to date to include the whole end day, got %v", to)
	}
	if dateInRange(mustDate(t, "2026-05-01"), from, to) {
		t.Fatal("expected range to exclude day after the to bound")
	}
}

func mustDate(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}
