package agent

import (
	"fmt"
	"strings"

	"github.com/henrygd/beszel/internal/entities/container"
)

// containerNameTemplate builds a container name from its Docker labels.
//
// CONTAINER_NAME_LABEL accepts either a plain label key ("coolify.serviceName") or a template
// that references labels with ${key} and may mix in literal text
// ("${coolify.projectName} - ${coolify.serviceName}"). Write $$ for a literal dollar sign.
type containerNameTemplate struct {
	parts []nameTemplatePart
}

type nameTemplatePart struct {
	literal string
	key     string // when set, the part is replaced by the value of this label
}

func parseContainerNameTemplate(value string) (*containerNameTemplate, error) {
	value = strings.TrimSpace(value)
	if !strings.Contains(value, "$") {
		return &containerNameTemplate{parts: []nameTemplatePart{{key: value}}}, nil
	}
	var parts []nameTemplatePart
	var literal strings.Builder
	flush := func() {
		if literal.Len() > 0 {
			parts = append(parts, nameTemplatePart{literal: literal.String()})
			literal.Reset()
		}
	}
	for i := 0; i < len(value); i++ {
		if value[i] != '$' {
			literal.WriteByte(value[i])
			continue
		}
		rest := value[i+1:]
		switch {
		case strings.HasPrefix(rest, "$"):
			literal.WriteByte('$')
			i++
		case strings.HasPrefix(rest, "{"):
			end := strings.IndexByte(rest, '}')
			if end < 0 {
				return nil, fmt.Errorf("unterminated ${ at position %d", i)
			}
			key := strings.TrimSpace(rest[1:end])
			if key == "" {
				return nil, fmt.Errorf("empty label name in ${} at position %d", i)
			}
			flush()
			parts = append(parts, nameTemplatePart{key: key})
			i += end + 1
		default:
			return nil, fmt.Errorf("use ${label} to reference a label (or $$ for a literal $) at position %d", i)
		}
	}
	flush()
	return &containerNameTemplate{parts: parts}, nil
}

// render returns the name for a container's labels. It reports false when any referenced
// label is missing or empty, so the caller can keep the Docker name instead of a partial one.
func (t *containerNameTemplate) render(labels map[string]string) (string, bool) {
	var b strings.Builder
	for _, part := range t.parts {
		if part.key == "" {
			b.WriteString(part.literal)
			continue
		}
		v := strings.TrimSpace(labels[part.key])
		if v == "" {
			return "", false
		}
		b.WriteString(v)
	}
	name := strings.TrimSpace(b.String())
	return name, name != ""
}

// resolveContainerNames sets DisplayName on each container. The rendered name is used when it is
// complete and unique among the reported containers. Containers that render to the same name
// (replicas) keep their Docker name so their series are never merged. tmpl must not be nil.
func resolveContainerNames(list []*container.ApiInfo, tmpl *containerNameTemplate, excluded func(string) bool) {
	candidates := make(map[*container.ApiInfo]string, len(list))
	fromLabel := make(map[*container.ApiInfo]bool, len(list))
	for _, ctr := range list {
		dockerName := ctr.Names[0][1:]
		ctr.DisplayName = dockerName
		if excluded(dockerName) {
			continue
		}
		candidates[ctr] = dockerName
		if name, ok := tmpl.render(ctr.Labels); ok {
			candidates[ctr] = name
			fromLabel[ctr] = true
		}
	}
	// Revert label names that are not unique. A reverted Docker name can collide with
	// another label name, so repeat until nothing changes.
	for changed := true; changed; {
		changed = false
		counts := make(map[string]int, len(candidates))
		for _, name := range candidates {
			counts[name]++
		}
		for ctr := range fromLabel {
			if counts[candidates[ctr]] > 1 {
				candidates[ctr] = ctr.Names[0][1:]
				delete(fromLabel, ctr)
				changed = true
			}
		}
	}
	for ctr, name := range candidates {
		ctr.DisplayName = name
	}
}
