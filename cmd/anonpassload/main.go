package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

type result struct {
	name string
	dur  time.Duration
	ok   bool
}

func main() {
	baseURL := flag.String("url", "http://127.0.0.1:8080", "anonpass base URL")
	clients := flag.Int("clients", 1000, "number of client accounts")
	tokens := flag.Int("tokens", 2, "tokens per client")
	requests := flag.Int("requests", 0, "random request count; 0 runs fixed clients*tokens flow")
	issueRate := flag.Float64("issue-rate", 0.55, "random mode issue probability")
	replayRate := flag.Float64("replay-rate", 0.10, "random mode replay probability")
	concurrency := flag.Int("concurrency", 100, "max concurrent requests")
	replay := flag.Bool("replay", true, "try replay after first redemption")
	seed := flag.Int64("seed", time.Now().UnixNano(), "random seed")
	flag.Parse()

	httpClient := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        *concurrency * 2,
			MaxIdleConnsPerHost: *concurrency * 2,
			IdleConnTimeout:     30 * time.Second,
		},
	}
	jobs := make(chan func() result)
	results := make(chan result, (*clients)*(*tokens)*3)

	var wg sync.WaitGroup
	for i := 0; i < *concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				results <- job()
			}
		}()
	}

	var issued atomic.Int64
	var redeemed atomic.Int64
	var replayRejected atomic.Int64
	var failed atomic.Int64

	start := time.Now()
	stats := loadStats{
		issued:         &issued,
		redeemed:       &redeemed,
		replayRejected: &replayRejected,
		failed:         &failed,
	}
	if *requests > 0 {
		go enqueueRandom(jobs, httpClient, *baseURL, *clients, *requests, *issueRate, *replayRate, *seed, stats)
	} else {
		go enqueueBatch(jobs, httpClient, *baseURL, *clients, *tokens, *replay, stats)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	var samples []time.Duration
	for r := range results {
		if r.dur > 0 {
			samples = append(samples, r.dur)
		}
	}
	elapsed := time.Since(start)
	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })

	if *requests > 0 {
		fmt.Printf("clients=%d requests=%d concurrency=%d issue_rate=%.2f replay_rate=%.2f seed=%d\n",
			*clients, *requests, *concurrency, *issueRate, *replayRate, *seed)
	} else {
		fmt.Printf("clients=%d tokens_per_client=%d concurrency=%d\n", *clients, *tokens, *concurrency)
	}
	fmt.Printf("issued=%d redeemed=%d replay_rejected=%d failed=%d\n", issued.Load(), redeemed.Load(), replayRejected.Load(), failed.Load())
	fmt.Printf("elapsed=%s throughput=%.1f tokens/s p50=%s p95=%s p99=%s\n",
		elapsed.Round(time.Millisecond),
		float64(redeemed.Load())/elapsed.Seconds(),
		percentile(samples, 50),
		percentile(samples, 95),
		percentile(samples, 99),
	)
}

type loadStats struct {
	issued         *atomic.Int64
	redeemed       *atomic.Int64
	replayRejected *atomic.Int64
	failed         *atomic.Int64
}

type clientState struct {
	mu       sync.Mutex
	account  string
	pending  []string
	redeemed []string
}

func enqueueBatch(jobs chan<- func() result, httpClient *http.Client, baseURL string, clients, tokens int, replay bool, stats loadStats) {
	for i := 0; i < clients; i++ {
		account := fmt.Sprintf("client-%06d@example.com", i)
		for j := 0; j < tokens; j++ {
			account := account
			jobs <- func() result {
				sessionID, r := issue(httpClient, baseURL, account)
				if !r.ok {
					stats.failed.Add(1)
					return r
				}
				stats.issued.Add(1)

				rr := redeem(httpClient, baseURL, sessionID, "redeem")
				if rr.ok {
					stats.redeemed.Add(1)
				} else {
					stats.failed.Add(1)
					return rr
				}

				if replay {
					rp := redeem(httpClient, baseURL, sessionID, "replay")
					if !rp.ok {
						stats.replayRejected.Add(1)
					}
					return rp
				}
				return result{name: fmt.Sprintf("client-token-%d", j), ok: true}
			}
		}
	}
	close(jobs)
}

func enqueueRandom(jobs chan<- func() result, httpClient *http.Client, baseURL string, clientCount, requestCount int, issueRate, replayRate float64, seed int64, stats loadStats) {
	rng := rand.New(rand.NewSource(seed))
	var rngMu sync.Mutex
	clients := make([]*clientState, clientCount)
	for i := range clients {
		clients[i] = &clientState{account: fmt.Sprintf("client-%06d@example.com", i)}
	}

	for i := 0; i < requestCount; i++ {
		jobs <- func() result {
			rngMu.Lock()
			client := clients[rng.Intn(len(clients))]
			roll := rng.Float64()
			rngMu.Unlock()

			switch {
			case roll < replayRate:
				return randomReplay(httpClient, baseURL, client, stats)
			case roll < replayRate+issueRate:
				return randomIssue(httpClient, baseURL, client, stats)
			default:
				return randomRedeem(httpClient, baseURL, client, stats)
			}
		}
	}
	close(jobs)
}

func randomIssue(httpClient *http.Client, baseURL string, client *clientState, stats loadStats) result {
	sessionID, r := issue(httpClient, baseURL, client.account)
	if !r.ok {
		stats.failed.Add(1)
		return r
	}

	client.mu.Lock()
	client.pending = append(client.pending, sessionID)
	client.mu.Unlock()

	stats.issued.Add(1)
	return r
}

func randomRedeem(httpClient *http.Client, baseURL string, client *clientState, stats loadStats) result {
	client.mu.Lock()
	if len(client.pending) == 0 {
		client.mu.Unlock()
		return randomIssue(httpClient, baseURL, client, stats)
	}
	sessionID := client.pending[0]
	client.pending = client.pending[1:]
	client.mu.Unlock()

	r := redeem(httpClient, baseURL, sessionID, "redeem")
	if !r.ok {
		stats.failed.Add(1)
		return r
	}

	client.mu.Lock()
	client.redeemed = append(client.redeemed, sessionID)
	client.mu.Unlock()

	stats.redeemed.Add(1)
	return r
}

func randomReplay(httpClient *http.Client, baseURL string, client *clientState, stats loadStats) result {
	client.mu.Lock()
	if len(client.redeemed) == 0 {
		client.mu.Unlock()
		return randomIssue(httpClient, baseURL, client, stats)
	}
	sessionID := client.redeemed[len(client.redeemed)-1]
	client.mu.Unlock()

	r := redeem(httpClient, baseURL, sessionID, "replay")
	if !r.ok {
		stats.replayRejected.Add(1)
		return r
	}
	stats.failed.Add(1)
	return r
}

func issue(client *http.Client, baseURL, account string) (string, result) {
	start := time.Now()
	var out struct {
		ID string `json:"id"`
	}
	err := postJSON(client, baseURL+"/v1/demo/issue", map[string]string{
		"account": account,
	}, &out)
	return out.ID, result{name: "issue", dur: time.Since(start), ok: err == nil}
}

func redeem(client *http.Client, baseURL, sessionID, name string) result {
	start := time.Now()
	var out map[string]any
	err := postJSON(client, baseURL+"/v1/demo/redeem", map[string]string{
		"session_id": sessionID,
	}, &out)
	return result{name: name, dur: time.Since(start), ok: err == nil}
}

func postJSON(client *http.Client, url string, body any, out any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	res, err := client.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer res.Body.Close()

	payload, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("%s: %s", res.Status, string(payload))
	}
	if out != nil {
		return json.Unmarshal(payload, out)
	}
	return nil
}

func percentile(samples []time.Duration, p int) time.Duration {
	if len(samples) == 0 {
		return 0
	}
	if p < 0 || p > 100 {
		log.Fatalf("bad percentile: %d", p)
	}
	idx := (len(samples) - 1) * p / 100
	return samples[idx].Round(time.Millisecond)
}
