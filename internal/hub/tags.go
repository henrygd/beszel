package hub

import (
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

// bindTagsEvents normalizes tag records. Removing a deleted tag's id from records
// that reference it is handled by PocketBase for any non-required relation field.
func bindTagsEvents(hub *Hub) {
	// trim tag names before create/update so the DB's case-insensitive unique
	// index doesn't get bypassed by incidental leading/trailing whitespace
	trimTagName := func(e *core.RecordEvent) error {
		e.Record.Set("name", strings.TrimSpace(e.Record.GetString("name")))
		return e.Next()
	}
	hub.OnRecordCreate("tags").BindFunc(trimTagName)
	hub.OnRecordUpdate("tags").BindFunc(trimTagName)
}
