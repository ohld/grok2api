package gateway

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"testing"
)

func mp4Box(boxType string, payload []byte) []byte {
	head := make([]byte, 8)
	binary.BigEndian.PutUint32(head, uint32(8+len(payload)))
	copy(head[4:], boxType)
	return append(head, payload...)
}

func mp4LargeBox(boxType string, payload []byte) []byte {
	head := make([]byte, 16)
	binary.BigEndian.PutUint32(head, 1)
	copy(head[4:], boxType)
	binary.BigEndian.PutUint64(head[8:], uint64(16+len(payload)))
	return append(head, payload...)
}

func TestMdatHasherIgnoresPerFileBoxes(t *testing.T) {
	video := bytes.Repeat([]byte("h264"), 1000)
	want := hex.EncodeToString(func() []byte { s := sha256.Sum256(video); return s[:] }())
	a := bytes.Join([][]byte{mp4Box("ftyp", []byte("isom")), mp4Box("uuid", []byte("id-one")), mp4Box("moov", make([]byte, 40)), mp4Box("mdat", video)}, nil)
	b := bytes.Join([][]byte{mp4Box("ftyp", []byte("isom")), mp4Box("uuid", []byte("id-two-different")), mp4Box("moov", make([]byte, 40)), mp4LargeBox("mdat", video)}, nil)
	for name, file := range map[string][]byte{"a": a, "b": b} {
		h := newMdatHasher()
		// Feed in odd-sized chunks so headers and payloads straddle writes.
		for i := 0; i < len(file); i += 7 {
			end := min(i+7, len(file))
			if _, err := h.Write(file[i:end]); err != nil {
				t.Fatal(err)
			}
		}
		if got := h.Fingerprint(); got != want {
			t.Fatalf("%s fingerprint = %s, want %s", name, got, want)
		}
	}
	if h := newMdatHasher(); h.Fingerprint() != "" {
		t.Fatal("empty stream must not fingerprint")
	}
	h := newMdatHasher()
	_, _ = h.Write([]byte("not an mp4 at all"))
	if h.Fingerprint() != "" {
		t.Fatal("garbage must not fingerprint")
	}
}

func TestVideoDecoyRegistryFlagsSameClipForDifferentPrompts(t *testing.T) {
	r := newVideoDecoyRegistry(2)
	if _, decoy := r.Observe("h1", "job-1", "a cat"); decoy {
		t.Fatal("first sighting is not a decoy")
	}
	if _, decoy := r.Observe("h1", "job-1", "a cat"); decoy {
		t.Fatal("re-observing the same job is not a decoy")
	}
	if _, decoy := r.Observe("h1", "job-2", "a cat"); decoy {
		t.Fatal("same prompt may legitimately render the same clip")
	}
	if prev, decoy := r.Observe("h1", "job-3", "a dog"); !decoy || prev != "job-1" {
		t.Fatalf("different prompt, same clip: decoy=%v prev=%s", decoy, prev)
	}
	if _, decoy := r.Observe("", "job-4", "a dog"); decoy {
		t.Fatal("missing fingerprint must never flag")
	}
	// Bounded: h1 is evicted after two newer fingerprints.
	r.Observe("h2", "job-5", "x")
	r.Observe("h3", "job-6", "y")
	if _, decoy := r.Observe("h1", "job-7", "z"); decoy {
		t.Fatal("evicted fingerprint must not flag")
	}
}
