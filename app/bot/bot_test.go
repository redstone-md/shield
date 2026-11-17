package bot

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDisplayName(t *testing.T) {
	tests := []struct {
		name     string
		msg      Message
		expected string
	}{
		{
			name: "DisplayName exists",
			msg: Message{
				From: User{
					ID:          1,
					Username:    "john",
					DisplayName: "John Doe",
				},
			},
			expected: "John Doe",
		},
		{
			name: "only Username exists",
			msg: Message{
				From: User{
					ID:       2,
					Username: "jane",
				},
			},
			expected: "jane",
		},
		{
			name: "only ID exists",
			msg: Message{
				From: User{
					ID: 3,
				},
			},
			expected: "3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, DisplayName(tt.msg))
		})
	}
}

func TestExternalReply_String(t *testing.T) {
	tbl := []struct {
		name string
		in   ExternalReply
		want string
	}{
		{name: "title and username", in: ExternalReply{ChatID: -100, Title: "Приват24", UserName: "privat24abu"},
			want: "«Приват24» (@privat24abu)"},
		{name: "title only", in: ExternalReply{ChatID: -100, Title: "Приват24"}, want: "«Приват24»"},
		{name: "id only", in: ExternalReply{ChatID: -100}, want: "chat -100"},
	}
	for _, tt := range tbl {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.in.String())
		})
	}
}
