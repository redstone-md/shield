package events

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_channelIDFromCallback(t *testing.T) {
	tests := []struct {
		name string
		id   int64
		want int64
	}{
		{"positive user ID", 12345, 0},
		{"zero ID", 0, 0},
		{"negative channel ID", -100123456, -100123456},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, channelIDFromCallback(tt.id))
		})
	}
}
