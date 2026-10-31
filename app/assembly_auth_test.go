package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveServerPassword(t *testing.T) {
	tests := []struct {
		name          string
		passwd, hash  string
		want          string
		wantGenerated bool
	}{
		{name: "auto generates", passwd: "auto", wantGenerated: true},
		{name: "auto with hash generates nothing", passwd: "auto", hash: "$2a$hash"},
		{name: "static", passwd: "secret", want: "secret"},
		{name: "static with hash uses the hash", passwd: "secret", hash: "$2a$hash"},
		{name: "empty", passwd: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveServerPassword(tt.passwd, tt.hash)
			require.NoError(t, err)
			if tt.wantGenerated {
				assert.Len(t, got, 20)
				return
			}
			assert.Equal(t, tt.want, got)
		})
	}
}
