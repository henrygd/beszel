//go:build testing

package agent

import (
	"encoding/json"
	"testing"

	"github.com/henrygd/beszel/internal/entities/container"
	"github.com/stretchr/testify/assert"
)

func testCtr(dockerName string, labels map[string]string) *container.ApiInfo {
	return &container.ApiInfo{Names: []string{"/" + dockerName}, Labels: labels}
}

func resolveNames(template string, excluded func(string) bool, ctrs ...*container.ApiInfo) []string {
	if excluded == nil {
		excluded = func(string) bool { return false }
	}
	var tmpl *containerNameTemplate
	if template != "" {
		var err error
		if tmpl, err = parseContainerNameTemplate(template); err != nil {
			panic(err)
		}
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

	t.Run("no label key keeps docker names", func(t *testing.T) {
		names := resolveNames("", nil, testCtr("api-abc", map[string]string{key: "api"}))
		assert.Equal(t, []string{"api-abc"}, names)
	})

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

func TestApiInfoLabelsDoNotLeakAcrossDecodes(t *testing.T) {
	// getDockerStats reuses the container slice between cycles
	list := []*container.ApiInfo{}
	first := `[{"Id":"1","Names":["/a"],"Labels":{"k":"v","only-first":"x"}}]`
	second := `[{"Id":"2","Names":["/a"],"Labels":{"k":"w"}}]`
	assert.NoError(t, json.Unmarshal([]byte(first), &list))
	list = resetContainerList(list)
	assert.NoError(t, json.Unmarshal([]byte(second), &list))
	assert.Equal(t, map[string]string{"k": "w"}, list[0].Labels)
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
