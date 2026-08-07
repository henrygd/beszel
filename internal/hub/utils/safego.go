package utils

import (
	"log/slog"
	"runtime/debug"
)

// SafeGo runs fn in a new goroutine, recovering from any panic it raises.
//
// A panic in a detached goroutine cannot be recovered by its parent and takes
// the whole process down with it. Background work such as metric collection or
// alert delivery is not worth crashing the hub over, so every detached
// goroutine that touches the database or the app should go through here.
//
// Panics are reported with the package-level slog rather than app.Logger()
// on purpose: PocketBase writes its logs to the database, and the most likely
// reason a background task panics is that the database is being torn down
// underneath it. stderr is the only sink that still works at that point.
func SafeGo(task string, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("recovered from panic in background task",
					"task", task,
					"panic", r,
					"stack", string(debug.Stack()),
				)
			}
		}()
		fn()
	}()
}
