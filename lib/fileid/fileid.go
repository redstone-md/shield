// Package fileid extracts a Telegram datacenter id from a Bot API file_id.
//
// Telegram does not expose a user's datacenter through the Bot API, but the
// file_id returned for a user's profile photo encodes the DC where that photo is
// stored. That DC is a stable, observable proxy for the user's home datacenter
// and is the only signal available without an MTProto client.
//
// The file_id format mirrors github.com/gotd/td/fileid (Bot API persistent id
// version 4). This package implements only the subset needed to read the DC; it
// does not decode id, access_hash, file_reference or photo_size_source.
package fileid

import (
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

const (
	// persistentIDVersion is the current Bot API file_id version. Versions 2
	// and 3 are legacy and no longer produced.
	persistentIDVersion = 4
	// typeID flag bits: web location (bit 24) and file reference (bit 25).
	webLocationFlag   = 1 << 24
	fileReferenceFlag = 1 << 25
	// lastType is one past the highest valid file type id.
	lastType = 18
)

// DecodeDC parses a Bot API file_id and returns the datacenter id stored in it.
//
// It returns an error for empty input, bad base64, an unsupported version, or a
// payload too short to carry the DC field. The DC sits at a fixed offset
// (immediately after the type id) regardless of flags or type, so the rest of
// the payload is not parsed.
func DecodeDC(fileID string) (int, error) {
	if fileID == "" {
		return 0, errors.New("file_id is empty")
	}
	raw, err := base64.RawURLEncoding.DecodeString(fileID)
	if err != nil {
		return 0, fmt.Errorf("base64 decode: %w", err)
	}
	data := rleDecode(raw)
	if len(data) < 2 {
		return 0, errors.New("decoded file_id is too short")
	}
	if version := data[len(data)-1]; version != persistentIDVersion {
		return 0, fmt.Errorf("unsupported file_id version %d", version)
	}
	body := data[:len(data)-1]
	// typeID (uint32 LE) + dcID (uint32 LE).
	if len(body) < 8 {
		return 0, io.ErrUnexpectedEOF
	}
	typeID := binary.LittleEndian.Uint32(body[0:4])
	typeID &^= webLocationFlag | fileReferenceFlag
	if typeID >= lastType {
		return 0, fmt.Errorf("unknown file_id type %d", typeID)
	}
	dc := int(binary.LittleEndian.Uint32(body[4:8]))
	if dc <= 0 {
		return 0, fmt.Errorf("invalid dc %d", dc)
	}
	return dc, nil
}

// rleDecode reverses Bot API's run-length encoding of zero bytes, matching
// github.com/gotd/td/fileid.rleDecode: a 0x00 byte signals that the next byte is
// the count of zero bytes to emit; all other bytes pass through unchanged.
func rleDecode(s []byte) []byte {
	var r []byte
	var last []byte
	for _, cur := range s {
		if len(last) == 1 && last[0] == 0 {
			r = append(r, make([]byte, int(cur))...)
			last = nil
			continue
		}
		r = append(r, last...)
		last = []byte{cur}
	}
	r = append(r, last...)
	return r
}
