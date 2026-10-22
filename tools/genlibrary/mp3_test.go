package main

import (
	"bytes"
	"testing"
)

func TestSyncsafe(t *testing.T) {
	for n, want := range map[int][]byte{
		0:         {0, 0, 0, 0},
		127:       {0, 0, 0, 127},
		128:       {0, 0, 1, 0},
		0xFFFFFFF: {127, 127, 127, 127},
	} {
		if got := syncsafe(n); !bytes.Equal(got, want) {
			t.Errorf("syncsafe(%d) = %v, want %v", n, got, want)
		}
	}
}

func TestSilenceLastsTheTrack(t *testing.T) {
	for _, secs := range []int{1, 31, 238} {
		frames := len(silence(secs)) / len(silentFrame)
		got := float64(frames*frameSamples) / sampleRate
		if got < float64(secs) || got >= float64(secs)+0.036 {
			t.Errorf("%d s of silence lasts %.3f s", secs, got)
		}
	}
}

func TestID3(t *testing.T) {
	tag := id3(tags{title: "Zoë", track: "1/5", year: 2015, compilation: true})
	if !bytes.HasPrefix(tag, []byte("ID3\x04\x00\x00")) {
		t.Fatalf("header %q", tag[:6])
	}
	size := int(tag[6])<<21 | int(tag[7])<<14 | int(tag[8])<<7 | int(tag[9])
	if size != len(tag)-10 {
		t.Errorf("tag size %d, body %d", size, len(tag)-10)
	}
	for _, want := range []string{"TIT2\x00\x00\x00\x05\x00\x00\x03Zoë", "TRCK", "TDRC", "TCMP"} {
		if !bytes.Contains(tag, []byte(want)) {
			t.Errorf("tag lacks %q", want)
		}
	}
	if bytes.Contains(tag, []byte("APIC")) {
		t.Error("a cover without one")
	}
}
