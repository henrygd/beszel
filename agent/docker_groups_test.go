//go:build testing

package agent

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"github.com/henrygd/beszel/internal/entities/container"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// group renders a resolved pair as "stack/service", for compact assertions.
func group(labels groupLabels) string {
	stack, service := labels.group()
	return stack + "/" + service
}

func TestGroupLabels(t *testing.T) {
	tests := []struct {
		name   string
		labels groupLabels
		want   string
	}{
		{
			name:   "swarm labels",
			labels: groupLabels{SwarmStack: "mentors", SwarmService: "mentors_web"},
			want:   "mentors/web",
		},
		{
			name:   "compose labels",
			labels: groupLabels{ComposeProject: "mentors", ComposeService: "web"},
			want:   "mentors/web",
		},
		{
			name:   "no labels",
			labels: groupLabels{},
			want:   "/",
		},
		{
			name:   "empty values fall back to compose",
			labels: groupLabels{SwarmStack: "", SwarmService: "", ComposeProject: "mentors", ComposeService: "web"},
			want:   "mentors/web",
		},
		{
			name:   "whitespace counts as missing and values are trimmed",
			labels: groupLabels{SwarmStack: "   ", ComposeProject: " mentors ", SwarmService: " mentors_api "},
			want:   "mentors/api",
		},
		{
			name: "swarm wins over compose",
			labels: groupLabels{
				SwarmStack: "mentors", SwarmService: "mentors_web",
				ComposeProject: "src", ComposeService: "frontend",
			},
			want: "mentors/web",
		},
		{
			name:   "service without the stack prefix is left alone",
			labels: groupLabels{SwarmStack: "mentors", SwarmService: "web"},
			want:   "mentors/web",
		},
		{
			name:   "stack without a service",
			labels: groupLabels{SwarmStack: "mentors"},
			want:   "mentors/",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, group(test.labels))
		})
	}
}

func TestGroupLabelsAreCapped(t *testing.T) {
	t.Run("an oversized label is cut to the cap", func(t *testing.T) {
		long := strings.Repeat("a", 300)
		stack, service := groupLabels{SwarmStack: long, SwarmService: long}.group()
		assert.Len(t, stack, maxGroupValueLength)
		assert.Len(t, service, maxGroupValueLength)
	})

	t.Run("the cut keeps valid utf-8", func(t *testing.T) {
		// two bytes per character, so a byte-wise cut would split the last one
		stack, _ := groupLabels{SwarmStack: strings.Repeat("ü", 300)}.group()
		assert.True(t, utf8.ValidString(stack))
		assert.Equal(t, maxGroupValueLength, utf8.RuneCountInString(stack))
	})

	t.Run("ordinary names are untouched", func(t *testing.T) {
		stack, service := groupLabels{SwarmStack: "mentors", SwarmService: "mentors_web"}.group()
		assert.Equal(t, "mentors", stack)
		assert.Equal(t, "web", service)
	})
}

func TestGroupLabelsFromMap(t *testing.T) {
	t.Run("reads the same four keys", func(t *testing.T) {
		labels := groupLabelsFromMap(map[string]string{
			labelSwarmStack:      "mentors",
			labelSwarmService:    "mentors_web",
			labelComposeProject:  "src",
			labelComposeService:  "frontend",
			"org.opencontainers": "noise",
		})
		assert.Equal(t, "mentors/web", group(labels))
	})

	t.Run("a nil map yields no group", func(t *testing.T) {
		assert.Equal(t, "/", group(groupLabelsFromMap(nil)))
	})
}

func TestTrimStackPrefix(t *testing.T) {
	tests := []struct {
		stack, service, want string
	}{
		{"mentors", "mentors_web", "web"},
		{"mentors", "web", "web"},
		{"mentors", "mentors_", "mentors_"},
		{"mentors", "mentorsweb", "mentorsweb"},
		{"mentors", "mentors", "mentors"},
		{"my_stack", "my_stack_web", "web"},
		{"", "mentors_web", "mentors_web"},
		{"mentors", "", ""},
	}
	for _, test := range tests {
		t.Run(test.stack+"+"+test.service, func(t *testing.T) {
			assert.Equal(t, test.want, trimStackPrefix(test.stack, test.service))
		})
	}
}

func TestDecodeContainerListGroups(t *testing.T) {
	swarm := `[{"Id":"1","Names":["/mentors_web.1.abc"],"Labels":{"com.docker.stack.namespace":"mentors","com.docker.swarm.service.name":"mentors_web","org.opencontainers.image.title":"noise"}}]`
	bare := `[{"Id":"2","Names":["/solo"],"Labels":{"org.opencontainers.image.title":"noise"}}]`

	t.Run("stack and service are resolved, the label map is not kept", func(t *testing.T) {
		dm := &dockerManager{}
		list := decodeList(t, dm, swarm)
		assert.Equal(t, "mentors", list[0].Stack)
		assert.Equal(t, "web", list[0].Service)
		assert.Nil(t, list[0].Labels, "the full label map costs an allocation per label and is not needed here")
	})

	t.Run("labels do not carry over between polls", func(t *testing.T) {
		dm := &dockerManager{}
		require.Equal(t, "mentors", decodeList(t, dm, swarm)[0].Stack)
		list := decodeList(t, dm, bare)
		assert.Equal(t, "2", list[0].Id)
		assert.Empty(t, list[0].Stack)
		assert.Empty(t, list[0].Service)
	})

	t.Run("a name template keeps the map and still resolves groups", func(t *testing.T) {
		tmpl, err := parseContainerNameTemplate("org.opencontainers.image.title")
		require.NoError(t, err)
		dm := &dockerManager{nameTemplate: tmpl}
		list := decodeList(t, dm, swarm)
		assert.Equal(t, "mentors", list[0].Stack)
		assert.Equal(t, "web", list[0].Service)
		assert.Equal(t, "noise", list[0].Labels["org.opencontainers.image.title"])
	})
}

func TestGetDockerStatsReportsContainerGroups(t *testing.T) {
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

	ctrJSON := func(id, name, stack string) string {
		return fmt.Sprintf(`{"Id":"%s","Names":["/%s"],"Image":"app:1","State":"running","Status":"Up 3 days","Labels":{"com.docker.stack.namespace":"%s","com.docker.swarm.service.name":"%s_web"}}`,
			id, name, stack, stack)
	}

	byID := func(stats []*container.Stats) map[string]string {
		out := make(map[string]string, len(stats))
		for _, s := range stats {
			out[s.Id] = s.Stack + "/" + s.Service + "/" + s.Name
		}
		return out
	}

	dm := newDockerManagerForVersionTest(server)
	dm.dockerVersionChecked = true
	dm.imageUpdatesDisabled = true

	setList("[" + ctrJSON("aaaaaaaaaaaa1111", "mentors_web.1.abc", "mentors") + "," +
		ctrJSON("bbbbbbbbbbbb2222", "mentors_web.2.def", "mentors") + "]")
	stats, err := dm.getDockerStats(defaultCacheTimeMs)
	require.NoError(t, err)
	// both replicas share a group, and their Docker names keep them apart
	assert.Equal(t, map[string]string{
		"aaaaaaaaaaaa": "mentors/web/mentors_web.1.abc",
		"bbbbbbbbbbbb": "mentors/web/mentors_web.2.def",
	}, byID(stats))

	// the stats records are reused between polls, so a container reporting another stack has to
	// move with it
	setList("[" + ctrJSON("aaaaaaaaaaaa1111", "tutors_web.1.abc", "tutors") + "]")
	stats, err = dm.getDockerStats(defaultCacheTimeMs)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"aaaaaaaaaaaa": "tutors/web/tutors_web.1.abc"}, byID(stats))

	t.Run("containers without the labels report no group", func(t *testing.T) {
		setList(`[{"Id":"cccccccccccc3333","Names":["/solo"],"Image":"nginx:1","State":"running","Status":"Up 3 days","Labels":{}}]`)
		stats, err := dm.getDockerStats(defaultCacheTimeMs)
		require.NoError(t, err)
		require.Len(t, stats, 1)
		assert.Empty(t, stats[0].Stack)
		assert.Empty(t, stats[0].Service)
		assert.Equal(t, "solo", stats[0].Name)
	})
}
