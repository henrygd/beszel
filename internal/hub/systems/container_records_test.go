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

func TestCreateContainerRecordsPersistsStackAndService(t *testing.T) {
	_, app := newTestSystemWithHub(t)

	const (
		systemID = "system123"
		webID    = "aaaaaaaaaaaa"
		soloID   = "bbbbbbbbbbbb"
	)

	type row struct {
		Name    string `db:"name"`
		Stack   string `db:"stack"`
		Service string `db:"service"`
	}
	read := func(t *testing.T, id string) row {
		t.Helper()
		var record row
		require.NoError(t, app.DB().Select("name", "stack", "service").From("containers").
			Where(dbx.HashExp{"id": id}).One(&record))
		return record
	}

	web := &container.Stats{Id: webID, Name: "mentors_web.1.kz9", Stack: "mentors", Service: "web"}
	solo := &container.Stats{Id: soloID, Name: "solo"}
	require.NoError(t, createContainerRecords(app, []*container.Stats{web, solo}, systemID))

	assert.Equal(t, row{Name: "mentors_web.1.kz9", Stack: "mentors", Service: "web"}, read(t, webID))
	assert.Equal(t, row{Name: "solo"}, read(t, soloID), "a container without the labels stores empty strings")

	// a redeploy into another stack reuses the row, so the upsert has to overwrite both fields
	web.Stack, web.Service = "tutors", "api"
	require.NoError(t, createContainerRecords(app, []*container.Stats{web}, systemID))
	assert.Equal(t, row{Name: "mentors_web.1.kz9", Stack: "tutors", Service: "api"}, read(t, webID))

	// and an agent that reports no group clears what is there, rather than leaving a stale stack
	web.Stack, web.Service = "", ""
	require.NoError(t, createContainerRecords(app, []*container.Stats{web}, systemID))
	assert.Equal(t, row{Name: "mentors_web.1.kz9"}, read(t, webID))
}
