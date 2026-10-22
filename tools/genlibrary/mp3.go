package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"strconv"
)

// A silent MP3 is one frame repeated: MPEG-2 Layer III, 8 kbit/s, 16 kHz,
// mono, no CRC. Its side information is all zeros, so every granule has no
// data and decodes to silence. Each frame is 36 bytes and lasts 576
// samples, 36 ms. Writing it directly keeps ffmpeg (and its ~90 packages)
// out of CI.
var silentFrame = func() []byte {
	f := make([]byte, 36)
	copy(f, []byte{0xFF, 0xF3, 0x18, 0xC0})
	return f
}()

const (
	frameSamples = 576
	sampleRate   = 16000
)

// silence is the audio for a track of the given length.
func silence(seconds int) []byte {
	frames := (seconds*sampleRate + frameSamples - 1) / frameSamples
	return bytes.Repeat(silentFrame, frames)
}

// tags are the ID3 frames genlibrary writes, in order.
type tags struct {
	title, artist, album, albumArtist string
	track, disc                       string // "n/total"
	year                              int
	genre                             string
	compilation                       bool
	cover                             []byte // a PNG, or nil
}

// id3 is an ID3v2.4 tag with UTF-8 text frames.
func id3(t tags) []byte {
	var body bytes.Buffer
	text := func(id, v string) {
		frame(&body, id, append([]byte{3}, v...)) // 3: UTF-8
	}
	text("TIT2", t.title)
	text("TPE1", t.artist)
	text("TALB", t.album)
	text("TPE2", t.albumArtist)
	text("TRCK", t.track)
	text("TPOS", t.disc)
	text("TDRC", strconv.Itoa(t.year))
	text("TCON", t.genre)
	if t.compilation {
		text("TCMP", "1")
	}
	if t.cover != nil {
		// Encoding, MIME type, picture type 3 (front cover), description.
		pic := append([]byte{3}, "image/png\x00"...)
		pic = append(pic, 3)
		pic = append(pic, "Album cover\x00"...)
		frame(&body, "APIC", append(pic, t.cover...))
	}
	head := []byte{'I', 'D', '3', 4, 0, 0}
	return append(append(head, syncsafe(body.Len())...), body.Bytes()...)
}

func frame(w *bytes.Buffer, id string, data []byte) {
	w.WriteString(id)
	w.Write(syncsafe(len(data)))
	w.Write([]byte{0, 0}) // flags
	w.Write(data)
}

// syncsafe is n in four bytes of seven bits each, as ID3v2.4 sizes are.
func syncsafe(n int) []byte {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], uint32(n&0x7F|n<<1&0x7F00|n<<2&0x7F0000|n<<3&0x7F000000))
	return b[:]
}

// writeMP3 writes a tagged silent MP3 of the given length.
func writeMP3(path string, t tags, seconds int) error {
	if seconds <= 0 {
		return fmt.Errorf("%s: a track must last at least a second", path)
	}
	return os.WriteFile(path, append(id3(t), silence(seconds)...), 0o644)
}
