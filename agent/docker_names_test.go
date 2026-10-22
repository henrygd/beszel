//go:build testing

package agent

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/henrygd/beszel/internal/entities/container"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testCtr(dockerName string, labels map[string]string) *container.ApiInfo {
	return &container.ApiInfo{Names: []string{"/" + dockerName}, Labels: labels}
}

func resolveNames(template string, excluded func(string) bool, ctrs ...*container.ApiInfo) []string {
	if excluded == nil {
		excluded = func(string) bool { return false }
	}
	tmpl, err := parseContainerNameTemplate(template)
	if err != nil {
		panic(err)
	}
	resolveContainerNames(ctrs, tmpl, excluded)
	names := make([]string, len(ctrs))
	for i, c := range ctrs {
		names[i] = c.DisplayName
	}
	return names
}

func TestResolveContainerNames(t *testing.T) {
	const key = "coolify.serviceName"

	t.Run("label value replaces docker name", func(t *testing.T) {
		names := resolveNames(key, nil,
			testCtr("api-abc", map[string]string{key: "api"}),
			testCtr("db-def", map[string]string{key: "db"}))
		assert.Equal(t, []string{"api", "db"}, names)
	})

	t.Run("missing, empty and whitespace labels fall back", func(t *testing.T) {
		names := resolveNames(key, nil,
			testCtr("a", nil),
			testCtr("b", map[string]string{key: ""}),
			testCtr("c", map[string]string{key: "   "}),
			testCtr("d", map[string]string{"other": "x"}))
		assert.Equal(t, []string{"a", "b", "c", "d"}, names)
	})

	t.Run("value is trimmed", func(t *testing.T) {
		names := resolveNames(key, nil, testCtr("a", map[string]string{key: "  api "}))
		assert.Equal(t, []string{"api"}, names)
	})

	t.Run("replicas sharing a value keep docker names", func(t *testing.T) {
		names := resolveNames(key, nil,
			testCtr("web-1", map[string]string{key: "web"}),
			testCtr("web-2", map[string]string{key: "web"}),
			testCtr("api-x", map[string]string{key: "api"}))
		assert.Equal(t, []string{"web-1", "web-2", "api"}, names)
	})

	t.Run("label value matching another docker name does not collide", func(t *testing.T) {
		names := resolveNames(key, nil,
			testCtr("api", nil),
			testCtr("api-x1", map[string]string{key: "api"}))
		assert.Equal(t, []string{"api", "api-x1"}, names)
	})

	t.Run("reverted docker name colliding with another label value is also reverted", func(t *testing.T) {
		names := resolveNames(key, nil,
			testCtr("b", map[string]string{key: "a"}),
			testCtr("c", map[string]string{key: "a"}),
			testCtr("a", map[string]string{key: "b"}))
		// "a" label is shared so b and c keep docker names; third wants "b" which now collides with docker name "b"
		assert.Equal(t, []string{"b", "c", "a"}, names)
	})

	t.Run("excluded containers do not take part in uniqueness checks", func(t *testing.T) {
		excluded := func(n string) bool { return n == "web-hidden" }
		names := resolveNames(key, excluded,
			testCtr("web-hidden", map[string]string{key: "web"}),
			testCtr("web-1", map[string]string{key: "web"}))
		assert.Equal(t, "web", names[1])
	})
}

func decodeList(t *testing.T, dm *dockerManager, body string) []*container.ApiInfo {
	t.Helper()
	resp := &http.Response{Body: io.NopCloser(strings.NewReader(body))}
	require.NoError(t, dm.decodeContainerList(resp))
	return dm.apiContainerList
}

func TestDecodeContainerListLabels(t *testing.T) {
	// getDockerStats reuses the container slice between cycles
	first := `[{"Id":"1","Names":["/a"],"Labels":{"k":"v","only-first":"x"}}]`
	second := `[{"Id":"2","Names":["/a"],"Labels":{"k":"w"}}]`

	t.Run("labels are not decoded without a template", func(t *testing.T) {
		dm := &dockerManager{}
		list := decodeList(t, dm, first)
		assert.Equal(t, "1", list[0].Id)
		assert.Nil(t, list[0].Labels)
	})

	t.Run("labels are decoded with a template and do not carry over", func(t *testing.T) {
		tmpl, err := parseContainerNameTemplate("k")
		require.NoError(t, err)
		dm := &dockerManager{nameTemplate: tmpl}
		list := decodeList(t, dm, first)
		assert.Equal(t, map[string]string{"k": "v", "only-first": "x"}, list[0].Labels)
		list = decodeList(t, dm, second)
		assert.Equal(t, "2", list[0].Id)
		assert.Equal(t, map[string]string{"k": "w"}, list[0].Labels)
	})

	t.Run("list size changes between polls keep the right labels", func(t *testing.T) {
		tmpl, err := parseContainerNameTemplate("k")
		require.NoError(t, err)
		dm := &dockerManager{nameTemplate: tmpl}
		two := `[{"Id":"1","Names":["/a"],"Labels":{"k":"a"}},{"Id":"2","Names":["/b"],"Labels":{"k":"b"}}]`
		one := `[{"Id":"3","Names":["/c"],"Labels":{"k":"c"}}]`
		list := decodeList(t, dm, two)
		require.Len(t, list, 2)
		list = decodeList(t, dm, one)
		require.Len(t, list, 1)
		assert.Equal(t, map[string]string{"k": "c"}, list[0].Labels)
		list = decodeList(t, dm, two)
		require.Len(t, list, 2)
		assert.Equal(t, map[string]string{"k": "a"}, list[0].Labels)
		assert.Equal(t, map[string]string{"k": "b"}, list[1].Labels)
	})
}

func TestGetDockerStatsNamesContainers(t *testing.T) {
	var mu sync.Mutex
	listBody := `[]`
	setList := func(body string) {
		mu.Lock()
		defer mu.Unlock()
		listBody = body
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.EscapedPath()
		switch {
		case path == "/containers/json":
			mu.Lock()
			body := listBody
			mu.Unlock()
			fmt.Fprint(w, body)
		case strings.HasPrefix(path, "/containers/") && strings.HasSuffix(path, "/stats"):
			fmt.Fprint(w, `{"memory_stats":{"usage":1048576}}`)
		default:
			t.Errorf("unexpected path: %s", path)
		}
	}))
	defer server.Close()

	initialList := `[
		{"Id":"aaaaaaaaaaaa1111","Names":["/app-api-1"],"Image":"app:1","State":"running","Status":"Up 3 days","Labels":{"coolify.serviceName":"api"}},
		{"Id":"bbbbbbbbbbbb2222","Names":["/db-redeploy-x"],"Image":"db:1","State":"running","Status":"Up 3 days","Labels":{"coolify.serviceName":"db"}},
		{"Id":"cccccccccccc3333","Names":["/hidden-1"],"Image":"hidden:1","State":"running","Status":"Up 3 days","Labels":{"coolify.serviceName":"hidden"}}
	]`

	namesByID := func(stats []*container.Stats) map[string]string {
		out := make(map[string]string, len(stats))
		for _, s := range stats {
			out[s.Id] = s.Name
		}
		return out
	}

	t.Run("label names are reported and follow renames", func(t *testing.T) {
		setList(initialList)
		dm := newDockerManagerForVersionTest(server)
		dm.dockerVersionChecked = true
		dm.imageUpdatesDisabled = true
		dm.excludeContainers = []string{"hidden*"}
		tmpl, err := parseContainerNameTemplate("coolify.serviceName")
		require.NoError(t, err)
		dm.nameTemplate = tmpl

		stats, err := dm.getDockerStats(defaultCacheTimeMs)
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"aaaaaaaaaaaa": "api", "bbbbbbbbbbbb": "db"}, namesByID(stats))

		// Redeploy: new Docker name suffix, and the label value changed
		setList(`[
			{"Id":"aaaaaaaaaaaa1111","Names":["/app-api-1"],"Image":"app:1","State":"running","Status":"Up 3 days","Labels":{"coolify.serviceName":"api"}},
			{"Id":"bbbbbbbbbbbb2222","Names":["/db-redeploy-y"],"Image":"db:1","State":"running","Status":"Up 3 days","Labels":{"coolify.serviceName":"database"}}
		]`)
		stats, err = dm.getDockerStats(defaultCacheTimeMs)
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"aaaaaaaaaaaa": "api", "bbbbbbbbbbbb": "database"}, namesByID(stats))
	})

	t.Run("docker names are reported without a template", func(t *testing.T) {
		setList(initialList)
		dm := newDockerManagerForVersionTest(server)
		dm.dockerVersionChecked = true
		dm.imageUpdatesDisabled = true
		dm.excludeContainers = []string{"hidden*"}

		stats, err := dm.getDockerStats(defaultCacheTimeMs)
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"aaaaaaaaaaaa": "app-api-1", "bbbbbbbbbbbb": "db-redeploy-x"}, namesByID(stats))
	})
}

func TestParseContainerNameTemplate(t *testing.T) {
	labels := map[string]string{"proj": "shop", "svc": "api", "empty": " ", "a.b-c_d": "dots"}
	render := func(tmpl string) (string, bool) {
		parsed, err := parseContainerNameTemplate(tmpl)
		assert.NoError(t, err)
		return parsed.render(labels)
	}

	t.Run("plain key", func(t *testing.T) {
		name, ok := render("svc")
		assert.True(t, ok)
		assert.Equal(t, "api", name)
	})
	t.Run("key with dots dashes and underscores", func(t *testing.T) {
		name, ok := render("${a.b-c_d}")
		assert.True(t, ok)
		assert.Equal(t, "dots", name)
	})
	t.Run("composite with literal text", func(t *testing.T) {
		name, ok := render("${proj} - ${svc}")
		assert.True(t, ok)
		assert.Equal(t, "shop - api", name)
	})
	t.Run("adjacent to hyphen without ambiguity", func(t *testing.T) {
		name, ok := render("${proj}-${svc}")
		assert.True(t, ok)
		assert.Equal(t, "shop-api", name)
	})
	t.Run("spaces inside braces are ignored", func(t *testing.T) {
		name, ok := render("${ proj }/${svc}")
		assert.True(t, ok)
		assert.Equal(t, "shop/api", name)
	})
	t.Run("$$ is a literal dollar", func(t *testing.T) {
		name, ok := render("$$${svc}")
		assert.True(t, ok)
		assert.Equal(t, "$api", name)
	})
	t.Run("missing or blank label rejects the whole name", func(t *testing.T) {
		_, ok := render("${proj} - ${nope}")
		assert.False(t, ok)
		_, ok = render("${proj} - ${empty}")
		assert.False(t, ok)
	})
	t.Run("invalid templates", func(t *testing.T) {
		for _, bad := range []string{"${proj", "${}", "$proj", "${proj} $"} {
			_, err := parseContainerNameTemplate(bad)
			assert.Error(t, err, bad)
		}
	})
}

func TestResolveContainerNamesComposite(t *testing.T) {
	const tmpl = "${project} - ${service}"
	l := func(p, s string) map[string]string { return map[string]string{"project": p, "service": s} }

	t.Run("same service in different projects stays distinct", func(t *testing.T) {
		names := resolveNames(tmpl, nil,
			testCtr("a-1", l("shop", "api")),
			testCtr("b-1", l("blog", "api")))
		assert.Equal(t, []string{"shop - api", "blog - api"}, names)
	})
	t.Run("replicas inside one project keep docker names", func(t *testing.T) {
		names := resolveNames(tmpl, nil,
			testCtr("a-1", l("shop", "api")),
			testCtr("a-2", l("shop", "api")),
			testCtr("c-1", l("shop", "db")))
		assert.Equal(t, []string{"a-1", "a-2", "shop - db"}, names)
	})
	t.Run("container missing one label keeps docker name", func(t *testing.T) {
		names := resolveNames(tmpl, nil,
			testCtr("x", map[string]string{"service": "api"}),
			testCtr("y", l("shop", "api")))
		assert.Equal(t, []string{"x", "shop - api"}, names)
	})
}
