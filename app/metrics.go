package main

import (
	"fmt"
	"net/http"
	"sort"
	"sync"
)

// A small Prometheus text-format registry, so the lab has no dependencies.
// Each metric writes its own # HELP/# TYPE header and samples.
type metric interface{ write(w http.ResponseWriter) }

var (
	regMu    sync.Mutex
	registry []metric
)

func register[M metric](m M) M {
	regMu.Lock()
	defer regMu.Unlock()
	registry = append(registry, m)
	return m
}

func metricsHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	regMu.Lock()
	defer regMu.Unlock()
	for _, m := range registry {
		m.write(w)
	}
}

type counterVec struct {
	mu         sync.Mutex
	name, help string
	label      string
	values     map[string]uint64
}

func newCounterVec(name, help, label string, seed ...string) *counterVec {
	c := &counterVec{name: name, help: help, label: label, values: map[string]uint64{}}
	for _, s := range seed {
		c.values[s] = 0 // Export zeros so rate() and ratios work from the start.
	}
	return register(c)
}

func (c *counterVec) inc(value string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.values[value]++
}

func (c *counterVec) write(w http.ResponseWriter) {
	c.mu.Lock()
	defer c.mu.Unlock()
	fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s counter\n", c.name, c.help, c.name)
	keys := make([]string, 0, len(c.values))
	for k := range c.values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(w, "%s{%s=%q} %d\n", c.name, c.label, k, c.values[k])
	}
}

type gauge struct {
	mu         sync.Mutex
	name, help string
	value      int64
}

func newGauge(name, help string) *gauge { return register(&gauge{name: name, help: help}) }

func (g *gauge) add(d int64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.value += d
}

func (g *gauge) write(w http.ResponseWriter) {
	g.mu.Lock()
	defer g.mu.Unlock()
	fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s gauge\n%s %d\n", g.name, g.help, g.name, g.name, g.value)
}

type histogram struct {
	mu         sync.Mutex
	name, help string
	bounds     []float64
	counts     []uint64 // Per bucket, not cumulative.
	sum        float64
	total      uint64
}

func newHistogram(name, help string, bounds []float64) *histogram {
	return register(&histogram{name: name, help: help, bounds: bounds, counts: make([]uint64, len(bounds))})
}

func (h *histogram) observe(seconds float64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.sum += seconds
	h.total++
	for i, b := range h.bounds {
		if seconds <= b {
			h.counts[i]++
			return
		}
	}
}

func (h *histogram) write(w http.ResponseWriter) {
	h.mu.Lock()
	defer h.mu.Unlock()
	fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s histogram\n", h.name, h.help, h.name)
	var cumulative uint64
	for i, b := range h.bounds {
		cumulative += h.counts[i]
		fmt.Fprintf(w, "%s_bucket{le=\"%g\"} %d\n", h.name, b, cumulative)
	}
	fmt.Fprintf(w, "%s_bucket{le=\"+Inf\"} %d\n%s_sum %g\n%s_count %d\n", h.name, h.total, h.name, h.sum, h.name, h.total)
}
