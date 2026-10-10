//go:build testing

package systems

import (
	"context"
	"testing"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// failingRecordHub fails record lookups with err, like a query that times out
// while the hub's VM is frozen.
type failingRecordHub struct {
	stubHub
	err error
}

func (h failingRecordHub) FindRecordById(collectionModelOrIdentifier any, recordId string, optFilters ...func(q *dbx.SelectQuery) error) (*core.Record, error) {
	return nil, h.err
}

func TestGetRecordKeepsSystemOnTransientError(t *testing.T) {
	sys, app := newTestSystemWithHub(t)
	sm := NewSystemManager(failingRecordHub{stubHub: stubHub{app}, err: context.DeadlineExceeded})
	t.Cleanup(sm.cancel)
	sys.manager = sm
	sys.ctx, sys.cancel = sys.getContext(sm.ctx)
	sm.systems.Set(sys.Id, sys)

	record, err := sys.getRecord(sm.hub)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Nil(t, record)
	assert.True(t, sm.HasSystem(sys.Id), "system should stay in the manager")
	assert.NoError(t, sys.ctx.Err(), "updater context should not be cancelled")
}

func TestGetRecordRemovesDeletedSystem(t *testing.T) {
	sys, app := newTestSystemWithHub(t)
	sm := NewSystemManager(stubHub{app})
	t.Cleanup(sm.cancel)
	sys.manager = sm
	sys.ctx, sys.cancel = sys.getContext(sm.ctx)
	sm.systems.Set(sys.Id, sys)

	record, err := app.FindRecordById("systems", sys.Id)
	require.NoError(t, err)
	require.NoError(t, app.Delete(record))

	_, err = sys.getRecord(sm.hub)
	require.Error(t, err)
	assert.False(t, sm.HasSystem(sys.Id), "deleted system should be removed")
	assert.Error(t, sys.ctx.Err(), "updater context should be cancelled")
}
