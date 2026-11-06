// A two-service shop used as a chaos engineering target.
// One binary, three roles, selected with ROLE: pricing, storefront, load.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"time"
)

var buckets = []float64{.025, .05, .1, .15, .25, .5, .75, 1, 1.5, 2.5, 5}

var (
	served = newCounterVec("shop_requests_total", "Requests served, by outcome.", "outcome",
		"ok", "fallback", "error", "abandoned_in_queue", "abandoned_in_flight")
	servedSeconds = newHistogram("shop_request_duration_seconds", "Server side latency.", buckets)
	inflight      = newGauge("shop_inflight_requests", "Checkouts holding a worker slot.")
	customer      = newCounterVec("customer_checkouts_total", "Checkouts as the customer saw them.", "result",
		"ok", "error", "gave_up")
	customerSeconds = newHistogram("customer_checkout_duration_seconds", "Latency as the customer saw it.", buckets)
)

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envDuration(key, fallback string) time.Duration {
	d, err := time.ParseDuration(env(key, fallback))
	if err != nil {
		log.Fatalf("%s: %v", key, err)
	}
	return d
}

func envInt(key, fallback string) int {
	n, err := strconv.Atoi(env(key, fallback))
	if err != nil {
		log.Fatalf("%s: %v", key, err)
	}
	return n
}

func healthz(w http.ResponseWriter, _ *http.Request) { fmt.Fprintln(w, "ok") }

// pricing returns a price after about 20ms of pretend work.
func pricing() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/price", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(time.Duration(15+rand.Intn(10)) * time.Millisecond)
		json.NewEncoder(w).Encode(map[string]float64{"price": 19.99})
	})
	return mux
}

// storefront serves /checkout with a fixed number of worker slots.
// PRICING_TIMEOUT=0 waits for pricing forever, which is the bug in this lab.
// FALLBACK=true answers from a last known price when pricing is slow.
func storefront() *http.ServeMux {
	pricingURL := env("PRICING_URL", "http://pricing:8080/price")
	timeout := envDuration("PRICING_TIMEOUT", "0s")
	fallback := env("FALLBACK", "false") == "true"
	slots := make(chan struct{}, envInt("MAX_INFLIGHT", "6"))
	client := &http.Client{Transport: &http.Transport{MaxIdleConnsPerHost: 64}}
	log.Printf("storefront: pricing timeout=%s fallback=%t slots=%d", timeout, fallback, cap(slots))

	callPricing := func(parent context.Context) error {
		ctx := parent
		if timeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(parent, timeout)
			defer cancel()
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, pricingURL, nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		_, err = io.Copy(io.Discard, resp.Body)
		return err
	}

	errorRate, _ := strconv.ParseFloat(env("ERROR_RATE", "0"), 64)
	cpuWork := envDuration("CPU_WORK", "0s")
	go leak(envInt("LEAK_MB_PER_MIN", "0"))

	mux := http.NewServeMux()
	mux.HandleFunc("/checkout", func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		defer func() { servedSeconds.observe(time.Since(start).Seconds()) }()

		select {
		case slots <- struct{}{}:
			defer func() { <-slots }()
		case <-r.Context().Done():
			served.inc("abandoned_in_queue")
			return
		}
		inflight.add(1)
		defer inflight.add(-1)

		burn(cpuWork)
		if rand.Float64() < errorRate {
			served.inc("error")
			http.Error(w, "injected error", http.StatusInternalServerError)
			return
		}

		err := callPricing(r.Context())
		switch {
		case err == nil:
			served.inc("ok")
			fmt.Fprintln(w, `{"status":"ok"}`)
		case errors.Is(r.Context().Err(), context.Canceled):
			served.inc("abandoned_in_flight")
		case fallback:
			served.inc("fallback")
			fmt.Fprintln(w, `{"status":"ok","price":"last_known"}`)
		default:
			served.inc("error")
			http.Error(w, "pricing unavailable", http.StatusBadGateway)
		}
	})
	return mux
}

// burn spins the CPU, so a request costs real compute.
func burn(d time.Duration) {
	for end := time.Now().Add(d); time.Now().Before(end); {
	}
}

// leak keeps memory forever, at a steady rate.
func leak(mbPerMin int) {
	if mbPerMin <= 0 {
		return
	}
	var kept [][]byte
	for range time.Tick(time.Second) {
		b := make([]byte, mbPerMin*1024*1024/60)
		for i := 0; i < len(b); i += 4096 {
			b[i] = 1
		}
		kept = append(kept, b)
	}
}

// load plays the customers: a steady arrival rate, and a customer
// who gives up after PATIENCE. Arrivals never wait for earlier requests.
func load() {
	target := env("TARGET_URL", "http://storefront:8080/checkout")
	rps := envInt("RPS", "20")
	client := &http.Client{
		Timeout:   envDuration("PATIENCE", "1s"),
		Transport: &http.Transport{MaxIdleConnsPerHost: 256},
	}
	log.Printf("load: %d requests/s against %s", rps, target)
	for range time.Tick(time.Second / time.Duration(rps)) {
		go func() {
			start := time.Now()
			resp, err := client.Get(target)
			customerSeconds.observe(time.Since(start).Seconds())
			switch {
			case err != nil:
				customer.inc("gave_up")
			case resp.StatusCode >= 500:
				customer.inc("error")
			default:
				customer.inc("ok")
			}
			if resp != nil {
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
			}
		}()
	}
}

func main() {
	var mux *http.ServeMux
	switch role := env("ROLE", ""); role {
	case "pricing":
		mux = pricing()
	case "storefront":
		mux = storefront()
	case "load":
		mux = http.NewServeMux()
		go load()
	default:
		log.Fatalf("ROLE must be pricing, storefront or load, got %q", role)
	}
	mux.HandleFunc("/healthz", healthz)
	mux.HandleFunc("/metrics", metricsHandler)
	log.Fatal(http.ListenAndServe(":"+env("PORT", "8080"), mux))
}
