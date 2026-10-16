//go:build testing

package hub

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	pbTests "github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/require"
)

func TestRestrictOwnershipUpdates(t *testing.T) {
	owner := `@request.auth.id != "" && user = @request.auth.id`
	guard := `@request.body.user:changed = false`
	for _, tc := range []struct {
		name, rule, expected string
		locked               bool
	}{
		{name: "locked", locked: true},
		{name: "public", rule: "", expected: guard},
		{name: "owner", rule: owner, expected: "(" + owner + ") && " + guard},
		{name: "custom disjunction", rule: owner + ` || user = "sentinel"`, expected: "(" + owner + ` || user = "sentinel") && ` + guard},
		{name: "already guarded", rule: "(" + owner + ") && " + guard, expected: "(" + owner + ") && " + guard},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app, err := pbTests.NewTestApp(t.TempDir())
			require.NoError(t, err)
			defer app.Cleanup()
			collection := core.NewBaseCollection("ownership_test")
			collection.Fields.Add(&core.TextField{Name: "user"})
			readRule := `user != ""`
			collection.ListRule = &readRule
			if !tc.locked {
				collection.UpdateRule = &tc.rule
			}
			require.NoError(t, app.Save(collection))
			for range 2 {
				require.NoError(t, restrictOwnershipUpdates(app, collection.Name, "user"))
				current, err := app.FindCollectionByNameOrId(collection.Id)
				require.NoError(t, err)
				if tc.locked {
					require.Nil(t, current.UpdateRule)
				} else {
					require.Equal(t, tc.expected, *current.UpdateRule)
				}
				require.Equal(t, readRule, *current.ListRule)
				require.Nil(t, current.CreateRule)
				require.Nil(t, current.DeleteRule)
			}
		})
	}
}
