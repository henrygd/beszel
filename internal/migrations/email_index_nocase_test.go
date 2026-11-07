//go:build testing

package migrations

import (
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/dbutils"
	"github.com/stretchr/testify/require"
)

func emailCollate(t *testing.T, app core.App, name string) string {
	c, err := app.FindCollectionByNameOrId(name)
	require.NoError(t, err)
	idx, ok := dbutils.FindSingleColumnUniqueIndex(c.Indexes, core.FieldNameEmail)
	require.True(t, ok)
	return strings.ToLower(idx.Columns[0].Collate)
}

// Existing case-variant duplicates must not break the migration; the index
// for that collection is left case-sensitive while others are still updated.
func TestEmailNocaseSkipsCollectionWithDuplicates(t *testing.T) {
	app, err := tests.NewTestApp(t.TempDir())
	require.NoError(t, err)
	defer app.Cleanup()

	require.NoError(t, setEmailIndexCollation(app, ""))
	require.Equal(t, "", emailCollate(t, app, "users"))

	users, err := app.FindCollectionByNameOrId("users")
	require.NoError(t, err)
	for _, email := range []string{"dup@example.com", "Dup@Example.com"} {
		r := core.NewRecord(users)
		r.SetEmail(email)
		r.SetPassword("password123")
		require.NoError(t, app.Save(r))
	}

	require.NoError(t, setEmailIndexCollation(app, "NOCASE"))
	require.Equal(t, "", emailCollate(t, app, "users"))
	require.Equal(t, "nocase", emailCollate(t, app, core.CollectionNameSuperusers))
}
