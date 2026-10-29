//go:build testing

package agent

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/henrygd/beszel/internal/entities/container"
)

// benchContainerListBody builds a /containers/json response with the shape of a busy Swarm node:
// containers entries, each carrying the Swarm and compose labels plus the usual noise from image
// builds and orchestrators.
func benchContainerListBody(containers, labelsPerContainer int) string {
	var b strings.Builder
	b.WriteByte('[')
	for i := range containers {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `{"Id":"%048d","Names":["/mentors_web.%d.abcdefghijkl"],"Image":"registry.example.com/mentors/web:1.2.3","State":"running","Status":"Up 3 days","Labels":{`, i, i)
		fmt.Fprintf(&b, `"%s":"mentors","%s":"mentors_web","%s":"mentors","%s":"web"`,
			labelSwarmStack, labelSwarmService, labelComposeProject, labelComposeService)
		for j := range labelsPerContainer - 4 {
			fmt.Fprintf(&b, `,"org.opencontainers.image.label%d":"value-%d-%d"`, j, i, j)
		}
		b.WriteString(`}}`)
	}
	b.WriteByte(']')
	return b.String()
}

// BenchmarkDecodeContainerList measures one /containers/json decode, which runs on every poll.
// group_labels is what every agent now does; label_map is the path a CONTAINER_NAME_LABEL template
// takes. labels_skipped decodes nothing but the entries, as a floor to compare against.
func BenchmarkDecodeContainerList(b *testing.B) {
	body := benchContainerListBody(50, 20)

	b.Run("labels_skipped", func(b *testing.B) {
		data := []byte(body)
		var list []*container.ApiInfo
		b.ReportAllocs()
		b.SetBytes(int64(len(body)))
		for b.Loop() {
			list = list[:0]
			if err := json.Unmarshal(data, &list); err != nil {
				b.Fatal(err)
			}
		}
	})

	cases := []struct {
		name string
		dm   func() *dockerManager
	}{
		{"group_labels", func() *dockerManager { return &dockerManager{} }},
		{"label_map", func() *dockerManager {
			tmpl, err := parseContainerNameTemplate(labelComposeService)
			if err != nil {
				panic(err)
			}
			return &dockerManager{nameTemplate: tmpl}
		}},
	}

	for _, c := range cases {
		b.Run(c.name, func(b *testing.B) {
			dm := c.dm()
			b.ReportAllocs()
			b.SetBytes(int64(len(body)))
			for b.Loop() {
				resp := &http.Response{Body: io.NopCloser(strings.NewReader(body))}
				if err := dm.decodeContainerList(resp); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
