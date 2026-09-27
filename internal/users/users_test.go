//go:build testing

package users_test

import (
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/henrygd/beszel/internal/migrations"
	beszelTests "github.com/henrygd/beszel/internal/tests"
	"github.com/henrygd/beszel/internal/users"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"
	"github.com/stretchr/testify/require"
)

type blockedBody struct {
	io.Reader
	entered chan struct{}
	resume  chan struct{}
}

func (b *blockedBody) Read(p []byte) (int, error) {
	if b.entered != nil {
		close(b.entered)
		b.entered = nil
		<-b.resume
	}
	return b.Reader.Read(p)
}

func TestCreateFirstUserAtomic(t *testing.T) {
	for _, scenario := range []string{"parked body", "concurrent requests", "rollback"} {
		t.Run(scenario, func(t *testing.T) {
			h, err := beszelTests.NewTestHub(t.TempDir())
			require.NoError(t, err)
			defer h.Cleanup()
			h.StartHub()
			um := users.NewUserManager(h.App)
			invoke := func(body io.Reader) int {
				req := httptest.NewRequest("POST", "/api/beszel/create-user", body)
				req.Header.Set("Content-Type", "application/json")
				res := httptest.NewRecorder()
				if err := um.CreateFirstUser(&core.RequestEvent{App: h.App, Event: router.Event{Request: req, Response: res}}); err != nil {
					t.Error(err)
				}
				return res.Code
			}
			body := func(email string) io.Reader {
				return strings.NewReader(`{"email":"` + email + `","password":"password12345"}`)
			}
			await := func(results <-chan int) int {
				select {
				case status := <-results:
					return status
				case <-time.After(10 * time.Second):
					t.Fatal("request did not finish")
					return 0
				}
			}
			switch scenario {
			case "parked body":
				entered, resume := make(chan struct{}), make(chan struct{})
				defer func() {
					select {
					case <-resume:
					default:
						close(resume)
					}
				}()
				result := make(chan int, 1)
				go func() { result <- invoke(&blockedBody{body("attacker@example.com"), entered, resume}) }()
				select {
				case <-entered:
				case <-time.After(10 * time.Second):
					t.Fatal("request did not reach body parsing")
				}
				require.Equal(t, 200, invoke(body("operator@example.com")))
				close(resume)
				require.Equal(t, 403, await(result))
			case "concurrent requests":
				start := make(chan struct{})
				results := make(chan int, 2)
				for _, email := range []string{"one@example.com", "two@example.com"} {
					go func() { <-start; results <- invoke(body(email)) }()
				}
				close(start)
				require.ElementsMatch(t, []int{200, 403}, []int{await(results), await(results)})
			case "rollback":
				hook := h.OnRecordCreate(core.CollectionNameSuperusers).BindFunc(func(e *core.RecordEvent) error {
					return errors.New("injected superuser creation failure")
				})
				require.Equal(t, 500, invoke(body("operator@example.com")))
				count, err := h.CountRecords("users")
				require.NoError(t, err)
				require.Zero(t, count)
				admins, err := h.FindAllRecords(core.CollectionNameSuperusers)
				require.NoError(t, err)
				require.Len(t, admins, 1)
				require.Equal(t, migrations.TempAdminEmail, admins[0].Email())
				h.OnRecordCreate(core.CollectionNameSuperusers).Unbind(hook)
				require.Equal(t, 200, invoke(body("operator@example.com")))
			}
			count, err := h.CountRecords("users")
			require.NoError(t, err)
			require.EqualValues(t, 1, count)
			bootstrapUsers, err := h.FindAllRecords("users")
			require.NoError(t, err)
			require.Equal(t, "admin", bootstrapUsers[0].GetString("role"))
			admins, err := h.FindAllRecords(core.CollectionNameSuperusers)
			require.NoError(t, err)
			require.Len(t, admins, 1)
			require.NotEqual(t, migrations.TempAdminEmail, admins[0].Email())
		})
	}
}
