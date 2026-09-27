//go:build testing

package systems

import (
	"testing"

	"github.com/henrygd/beszel/internal/entities/container"
	"github.com/pocketbase/dbx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateContainerRecordsPersistsImageUpdateAvailability(t *testing.T) {
	_, app := newTestSystemWithHub(t)

	const (
		systemID    = "system123"
		containerID = "abcdef123456"
		image       = "nginx:latest"
	)

	data := &container.Stats{
		Id:              containerID,
		Name:            "web",
		Image:           image,
		UpdateAvailable: true,
	}
	require.NoError(t, createContainerRecords(app, []*container.Stats{data}, systemID))

	var record struct {
		Image           string `db:"image"`
		UpdateAvailable bool   `db:"updatable"`
	}
	require.NoError(t, app.DB().Select("image", "updatable").From("containers").
		Where(dbx.HashExp{"id": containerID}).One(&record))
	assert.Equal(t, image, record.Image)
	assert.True(t, record.UpdateAvailable)

	data.UpdateAvailable = false
	require.NoError(t, createContainerRecords(app, []*container.Stats{data}, systemID))
	require.NoError(t, app.DB().Select("image", "updatable").From("containers").
		Where(dbx.HashExp{"id": containerID}).One(&record))
	assert.Equal(t, image, record.Image)
	assert.False(t, record.UpdateAvailable)
}
