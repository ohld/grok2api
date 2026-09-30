package gateway

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"hash"
	"sync"
)

// Degraded free Web accounts get a stock clip from a fixed pool (lighthouse,
// alpine lake, tabby cat, ...) that ignores the prompt but still spends the
// account's daily clip (upstream #1014, #954, #1053). Each file's SHA256
// differs because the MP4 carries a per-file uuid box, while the H.264
// payload in mdat is the same. Hashing only the mdat payload therefore
// fingerprints the pool clip without decoding video. Two jobs with different
// prompts and the same fingerprint can only be a pool clip.

var errVideoDecoy = errors.New("Grok returned a stock decoy clip")

// mdatHasher is an io.Writer that hashes the payload of every top-level mdat
// box while an MP4 streams through it.
type mdatHasher struct {
	sum     hash.Hash
	header  []byte // pending box header bytes (8 or 16)
	need    int    // header bytes still needed; 0 while inside a box
	remain  uint64 // payload bytes left in the current box
	hashing bool   // current box is mdat
	seen    bool   // at least one mdat byte hashed
	broken  bool   // unparseable input; fingerprint unusable
}

func newMdatHasher() *mdatHasher {
	return &mdatHasher{sum: sha256.New(), need: 8}
}

func (h *mdatHasher) Write(p []byte) (int, error) {
	n := len(p)
	if h.broken {
		return n, nil
	}
	for len(p) > 0 {
		if h.remain > 0 {
			take := uint64(len(p))
			if take > h.remain {
				take = h.remain
			}
			if h.hashing {
				h.sum.Write(p[:take])
				h.seen = true
			}
			h.remain -= take
			p = p[take:]
			if h.remain == 0 {
				h.need, h.header, h.hashing = 8, h.header[:0], false
			}
			continue
		}
		take := h.need
		if take > len(p) {
			take = len(p)
		}
		h.header = append(h.header, p[:take]...)
		h.need -= take
		p = p[take:]
		if h.need > 0 {
			continue
		}
		size := uint64(binary.BigEndian.Uint32(h.header[:4]))
		boxType := string(h.header[4:8])
		switch {
		case size == 1 && len(h.header) == 8:
			h.need = 8 // 64-bit largesize follows
			continue
		case size == 1:
			size = binary.BigEndian.Uint64(h.header[8:16])
		case size == 0:
			// "to end of file": only valid for the last box; treat the rest as payload.
			size = ^uint64(0)
		}
		if size < uint64(len(h.header)) {
			h.broken = true
			return n, nil
		}
		h.remain = size - uint64(len(h.header))
		h.hashing = boxType == "mdat"
		if h.remain == 0 {
			h.need, h.header = 8, h.header[:0]
		}
	}
	return n, nil
}

// Fingerprint is the hex SHA256 of the mdat payload, or "" when the stream
// had no mdat or did not parse as MP4.
func (h *mdatHasher) Fingerprint() string {
	if h.broken || !h.seen {
		return ""
	}
	return hex.EncodeToString(h.sum.Sum(nil))
}

// videoDecoyRegistry remembers recent clip fingerprints per prompt.
// ponytail: in-process and bounded; a restart forgets it. Persist known decoy
// fingerprints (they are logged as video_content_hash) if the pool proves stable.
type videoDecoyRegistry struct {
	mu      sync.Mutex
	entries map[string]videoDecoyEntry
	order   []string
	limit   int
}

type videoDecoyEntry struct {
	jobID  string
	prompt string
}

// videoDecoys returns the process-wide registry (lazily, so tests that build a
// Service by hand get one too).
func (s *Service) videoDecoys() *videoDecoyRegistry {
	s.videoDecoyOnce.Do(func() { s.videoDecoyRegistry = newVideoDecoyRegistry(4096) })
	return s.videoDecoyRegistry
}

func newVideoDecoyRegistry(limit int) *videoDecoyRegistry {
	return &videoDecoyRegistry{entries: map[string]videoDecoyEntry{}, limit: max(1, limit)}
}

// Observe records the clip and returns the earlier job's id when the same
// fingerprint was produced for a different prompt.
func (r *videoDecoyRegistry) Observe(fingerprint, jobID, prompt string) (string, bool) {
	if fingerprint == "" {
		return "", false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if previous, ok := r.entries[fingerprint]; ok {
		if previous.jobID != jobID && previous.prompt != prompt {
			return previous.jobID, true
		}
		return "", false
	}
	if len(r.order) >= r.limit {
		oldest := r.order[0]
		r.order = r.order[1:]
		delete(r.entries, oldest)
	}
	r.entries[fingerprint] = videoDecoyEntry{jobID: jobID, prompt: prompt}
	r.order = append(r.order, fingerprint)
	return "", false
}
