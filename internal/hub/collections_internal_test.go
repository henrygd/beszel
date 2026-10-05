//go:build testing

package hub

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
)

// TestCompareCollectionRulesNilSemantics pins the nil vs "" distinction: in
// PocketBase a nil rule means superusers-only while an empty rule means
// public, so swapping one for the other is security-relevant drift and must
// be flagged, never treated as "unchanged".
func TestCompareCollectionRulesNilSemantics(t *testing.T) {
	target := collectionRules{}

	col := core.NewCollection(core.CollectionTypeBase, "test")
	changed, drifted := compareCollectionRules(col, target)
	assert.False(t, changed, "nil rules matching nil targets are in sync")
	assert.Empty(t, drifted)

	empty := ""
	col.CreateRule = &empty
	changed, drifted = compareCollectionRules(col, target)
	assert.True(t, changed)
	assert.Contains(t, drifted, "createRule",
		"an empty (public) rule replacing a nil (superusers-only) rule is drift")
}
