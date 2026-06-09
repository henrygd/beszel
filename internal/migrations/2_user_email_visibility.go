package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		_, err := app.DB().NewQuery("UPDATE users SET emailVisibility = 1 WHERE emailVisibility = 0").Execute()
		return err
	}, nil)
}
