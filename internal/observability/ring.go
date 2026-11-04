package observability

import (
	"time"
)

// ringBuffer is a fixed-capacity FIFO of Records. It is the memory half of the
// logger: Snapshot reads it, the event path appends to it, and it keeps the
// newest records when it wraps. It is not safe for concurrent use on its own;
// Log guards it with its mutex.
type ringBuffer struct {
	buf   []Record
	start int // index of the oldest record
	n     int // number of live records
}

// newRingBuffer returns a ring that holds size records. A non-positive size
// falls back to ringSizeDefault so a zero Options never panics.
func newRingBuffer(size int) *ringBuffer {
	if size <= 0 {
		size = ringSizeDefault
	}
	return &ringBuffer{buf: make([]Record, size)}
}

// add appends rec, evicting the oldest record when the ring is full.
func (r *ringBuffer) add(rec Record) {
	if r.n < len(r.buf) {
		r.buf[(r.start+r.n)%len(r.buf)] = rec
		r.n++
		return
	}
	r.buf[r.start] = rec
	r.start = (r.start + 1) % len(r.buf)
}

// len returns how many records are remembered.
func (r *ringBuffer) len() int { return r.n }

// snapshot copies the remembered records, newest last. A zero since keeps
// everything; a positive limit keeps only the newest limit matches, a
// non-positive limit keeps all of them. It returns nil when nothing matches so
// callers can compare against nil.
func (r *ringBuffer) snapshot(since time.Time, limit int) []Record {
	out := make([]Record, 0, r.n)
	for i := 0; i < r.n; i++ {
		rec := r.buf[(r.start+i)%len(r.buf)]
		if !since.IsZero() && rec.Time.Before(since) {
			continue
		}
		out = append(out, rec)
	}
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
