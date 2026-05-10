package events

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/umputun/tg-spam/app/bot"
)

func Test_stickerDownloadFileID(t *testing.T) {
	tests := []struct {
		name string
		info *bot.StickerInfo
		want string
	}{
		{
			name: "static sticker uses thumbnail",
			info: &bot.StickerInfo{FileID: "file123", ThumbFileID: "thumb456"},
			want: "thumb456",
		},
		{
			name: "animated sticker uses thumbnail",
			info: &bot.StickerInfo{FileID: "anim_file", ThumbFileID: "anim_thumb", IsAnimated: true},
			want: "anim_thumb",
		},
		{
			name: "video sticker uses thumbnail",
			info: &bot.StickerInfo{FileID: "vid_file", ThumbFileID: "vid_thumb", IsVideo: true},
			want: "vid_thumb",
		},
		{
			name: "animated sticker without thumbnail falls back to FileID",
			info: &bot.StickerInfo{FileID: "anim_no_thumb", IsAnimated: true},
			want: "anim_no_thumb",
		},
		{
			name: "empty FileID and ThumbFileID",
			info: &bot.StickerInfo{},
			want: "",
		},
		{
			name: "no FileID but has ThumbFileID",
			info: &bot.StickerInfo{ThumbFileID: "only_thumb"},
			want: "only_thumb",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, stickerDownloadFileID(tt.info))
		})
	}
}
