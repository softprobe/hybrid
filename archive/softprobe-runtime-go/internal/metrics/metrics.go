// Package metrics provides lightweight Prometheus-compatible counters and
// histograms for the softprobe runtime. It emits text-format exposition
// without depending on the full prometheus/client_golang package.
package metrics

import (
	"fmt"
	"io"
	"math"
	"sort"
	"sync"
	"sync/atomic"
)

// Counter is a monotonically increasing uint64.
type Counter struct {
	v uint64
}

func (c *Counter) Inc() { atomic.AddUint64(&c.v, 1) }
func (c *Counter) Add(n uint64) { atomic.AddUint64(&c.v, n) }
func (c *Counter) Value() uint64 { return atomic.LoadUint64(&c.v) }

// LabeledCounters holds per-label counters under one metric family.
type LabeledCounters struct {
	mu      sync.Mutex
	label   string
	buckets map[string]*Counter
}

func NewLabeledCounters(label string) *LabeledCounters {
	return &LabeledCounters{label: label, buckets: make(map[string]*Counter)}
}

func (lc *LabeledCounters) Inc(value string) {
	lc.mu.Lock()
	c, ok := lc.buckets[value]
	if !ok {
		c = &Counter{}
		lc.buckets[value] = c
	}
	lc.mu.Unlock()
	c.Inc()
}

func (lc *LabeledCounters) snapshot() map[string]uint64 {
	lc.mu.Lock()
	defer lc.mu.Unlock()
	out := make(map[string]uint64, len(lc.buckets))
	for k, v := range lc.buckets {
		out[k] = v.Value()
	}
	return out
}

// Histogram approximates a Prometheus histogram with fixed buckets suitable
// for request latency in seconds.
type Histogram struct {
	mu      sync.Mutex
	bounds  []float64
	buckets []uint64
	sum     float64
	count   uint64
}

var defaultLatencyBounds = []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5}

func NewHistogram() *Histogram {
	bounds := defaultLatencyBounds
	return &Histogram{
		bounds:  bounds,
		buckets: make([]uint64, len(bounds)+1),
	}
}

func (h *Histogram) Observe(v float64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.sum += v
	h.count++
	for i, b := range h.bounds {
		if v <= b {
			h.buckets[i]++
			return
		}
	}
	h.buckets[len(h.bounds)]++ // +Inf bucket
}

// Registry holds all metrics families for the runtime.
type Registry struct {
	SessionsTotal      *LabeledCounters
	InjectTotal        *LabeledCounters
	InjectLatency      *Histogram
	ExtractSpansTotal  *Counter
}

var Global = &Registry{
	SessionsTotal:     NewLabeledCounters("mode"),
	InjectTotal:       NewLabeledCounters("result"),
	InjectLatency:     NewHistogram(),
	ExtractSpansTotal: &Counter{},
}

// WriteTo writes Prometheus text-format exposition to w.
func (r *Registry) WriteTo(w io.Writer) {
	writeCounter(w, "softprobe_sessions_total", "Total sessions created, by mode.", r.SessionsTotal)
	writeCounter(w, "softprobe_inject_requests_total", "Total inject requests, by result (hit, miss, error).", r.InjectTotal)
	writeHistogram(w, "softprobe_inject_latency_seconds", "Inject request latency in seconds.", r.InjectLatency)
	writeSingleCounter(w, "softprobe_extract_spans_total", "Total OTLP extract spans received.", r.ExtractSpansTotal.Value())
}

func writeCounter(w io.Writer, name, help string, lc *LabeledCounters) {
	fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s counter\n", name, help, name)
	snap := lc.snapshot()
	keys := make([]string, 0, len(snap))
	for k := range snap {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(w, "%s{%s=%q} %d\n", name, lc.label, k, snap[k])
	}
	if len(keys) == 0 {
		fmt.Fprintf(w, "%s 0\n", name)
	}
}

func writeSingleCounter(w io.Writer, name, help string, v uint64) {
	fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s counter\n", name, help, name)
	fmt.Fprintf(w, "%s %d\n", name, v)
}

func writeHistogram(w io.Writer, name, help string, h *Histogram) {
	fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s histogram\n", name, help, name)
	h.mu.Lock()
	defer h.mu.Unlock()
	var cum uint64
	for i, b := range h.bounds {
		cum += h.buckets[i]
		fmt.Fprintf(w, "%s_bucket{le=%q} %d\n", name, formatFloat(b), cum)
	}
	cum += h.buckets[len(h.bounds)]
	fmt.Fprintf(w, "%s_bucket{le=\"+Inf\"} %d\n", name, cum)
	fmt.Fprintf(w, "%s_sum %s\n", name, formatFloat(h.sum))
	fmt.Fprintf(w, "%s_count %d\n", name, h.count)
}

func formatFloat(f float64) string {
	if f == math.Trunc(f) {
		return fmt.Sprintf("%.1f", f)
	}
	return fmt.Sprintf("%g", f)
}
