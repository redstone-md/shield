package fileid

import (
	"encoding/base64"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// realBotAPIFileIDs are real-world Bot API file_ids with a known DC (2), taken
// from github.com/gotd/td/fileid test fixtures. They exercise base64, RLE,
// version, type-id flag clearing and the fixed DC offset against real payloads
// (including ones with file_reference and web-location/flag variants).
var realBotAPIFileIDs = map[string]string{
	"Sticker":         "CAACAgIAAxkBAAM6YZlDEHCmaTKrUhCIjxAPtPtjVx4AAicAA4dXjx6dGLyHwXVNcCIE",
	"AnimatedSticker": "CAACAgIAAxkBAANCYZzsGL3c2jB4BE46_bD9-aaYH10AApEOAAJZQylKSstCqeiyJ5giBA",
	"GIF":             "CgACAgIAAxkBAAM7YZqVjhoGXOIk6qgVu7xd0QvyRVEAArQQAAK7XrBIi5xgKHPRFpQiBA",
	"GIFThumbnail":    "AAMCAgADGQEAAzthmpWOGgZc4iTqqBW7vF3RC_JFUQACtBAAArtesEiLnGAoc9EWlAEAB20AAyIE",
	"Photo":           "AgACAgIAAxkBAAM9YZqXG-B0WHEv7lFlQxOQDs6jrGQAAoa7MRvdfNlIhJa73cDxR0kBAAMCAAN4AAMiBA",
	"Video":           "BAACAgIAAxkBAANAYZzjSkCVY7Ttrp2l92eCQzYYxVEAAkoRAAJIYKFIRionwJTz4kIiBA",
	"VideoThumbnail":  "AAMCAgADGQEAA0BhnONKQJVjtO2unaX3Z4JDNhjFUQACShEAAkhgoUhGKifAlPPiQgEAB20AAyIE",
	"ChatPhoto":       "AQADAgAD7a8xG75QcEkACAMAA2jAIuIW____cd7THMWjNdIiBA",
	"Voice":           "AwACAgIAAxkBAANDYZzsXw55-6fljCSeQXEP3dX5_egAAlkSAAJStulIAYO3JdIypKQiBA",
	"Audio":           "CQACAgIAAxkBAANEYZzt3rDAw5CkHSU8RZA8AzTTsyMAAvACAAKoAAF4SjhQUd8y3lIoIgQ",
}

func TestDecodeDC_RealBotAPIFileIDs(t *testing.T) {
	for name, id := range realBotAPIFileIDs {
		t.Run(name, func(t *testing.T) {
			dc, err := DecodeDC(id)
			require.NoError(t, err, "real file_id %q must decode", name)
			assert.Equal(t, 2, dc, "expected DC 2 for %s", name)
		})
	}
}

func TestDecodeDC_RoundTrip(t *testing.T) {
	// document type (5) carries no photo_size_source, so a minimal payload is
	// just [typeID][dcID][subVersion][version]. This validates arbitrary DCs
	// end-to-end through base64 + RLE + version + offset handling.
	cases := []struct {
		name  string
		fType uint32
		dc    int
	}{
		{"Document_DC1", 5, 1},
		{"Document_DC2", 5, 2},
		{"Document_DC4", 5, 4},
		{"ProfilePhoto_DC3", 1, 3}, // profile photo type, the user-avatar case
		{"Photo_DC5", 2, 5},
		{"Document_DC_with_high_byte", 5, 257}, // dc byte 1 is non-zero, exercises LE order
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			encoded := encodeTestFileID(tc.fType, tc.dc)
			got, err := DecodeDC(encoded)
			require.NoError(t, err)
			assert.Equal(t, tc.dc, got)
		})
	}
}

func TestDecodeDC_Errors(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"Empty", "", true},
		{"InvalidBase64", "/-*-/--+", true},
		{"TooSmall", base64.RawURLEncoding.EncodeToString([]byte{1}), true},
		// version 2 is legacy/unsupported.
		{"UnsupportedVersion", base64.RawURLEncoding.EncodeToString([]byte{1, 2}), true},
		// version 20 is unknown.
		{"UnknownVersion", base64.RawURLEncoding.EncodeToString([]byte{1, 20}), true},
		// valid version 4 but body too short to hold a DC field.
		{"BodyTooShort", encodeRawBytes([]byte{5, 4}), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := DecodeDC(tc.input)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// encodeTestFileID builds a minimal valid version-4 file_id for a non-photo type
// (no photo_size_source), mirroring gotd's encode path: body + version, RLE,
// base64 RawURL.
func encodeTestFileID(fType uint32, dc int) string {
	var buf [4]byte
	body := make([]byte, 0, 9)
	binary.LittleEndian.PutUint32(buf[:], fType)
	body = append(body, buf[:]...)
	binary.LittleEndian.PutUint32(buf[:], uint32(dc))
	body = append(body, buf[:]...)
	body = append(body, latestSubVersion, persistentIDVersion)
	return base64.RawURLEncoding.EncodeToString(rleEncode(body))
}

// encodeRawBytes applies RLE + base64 to arbitrary bytes (for error-case inputs
// that already include a version byte).
func encodeRawBytes(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(rleEncode(b))
}

// rleEncode mirrors github.com/gotd/td/fileid.rleEncode (run-length encode of
// zero bytes), used here only by the test encoder.
func rleEncode(s []byte) []byte {
	var r []byte
	var count byte
	for _, cur := range s {
		if cur == 0 {
			count++
			continue
		}
		if count > 0 {
			r = append(r, 0, count)
			count = 0
		}
		r = append(r, cur)
	}
	if count > 0 {
		r = append(r, 0, count)
	}
	return r
}

const latestSubVersion = 34
