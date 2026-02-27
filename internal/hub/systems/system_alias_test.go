package systems

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveAutoContainerAlias(t *testing.T) {
	t.Run("replaces variables from labels", func(t *testing.T) {
		alias := resolveAutoContainerAlias("$SERVICE-$ENV", map[string]string{
			"SERVICE": "api",
			"ENV":     "prod",
		})
		assert.Equal(t, "api-prod", alias)
	})

	t.Run("returns empty when a label is missing", func(t *testing.T) {
		alias := resolveAutoContainerAlias("$SERVICE-$ENV", map[string]string{
			"SERVICE": "api",
		})
		assert.Empty(t, alias)
	})

	t.Run("returns empty when template has no variables", func(t *testing.T) {
		alias := resolveAutoContainerAlias("plain-text", map[string]string{
			"SERVICE": "api",
		})
		assert.Empty(t, alias)
	})

	t.Run("returns empty when labels are missing", func(t *testing.T) {
		alias := resolveAutoContainerAlias("$SERVICE", nil)
		assert.Empty(t, alias)
	})
}
