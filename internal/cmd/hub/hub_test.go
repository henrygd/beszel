//go:build testing

package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsUpdateCmd(t *testing.T) {
	rootCmd := getBaseApp().RootCmd

	tests := []struct {
		name string
		args []string
		want bool
	}{
		{"update", []string{"update"}, true},
		{"update with flag", []string{"update", "--china-mirrors"}, true},
		{"update with global flag first", []string{"--dir", "/tmp/x", "update"}, true},
		{"update help", []string{"update", "--help"}, true},
		{"serve", []string{"serve"}, false},
		{"health", []string{"health", "--url", "http://localhost"}, false},
		{"no args", []string{}, false},
		{"unknown", []string{"nope"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isUpdateCmd(rootCmd, tt.args))
		})
	}
}
