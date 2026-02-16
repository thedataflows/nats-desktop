package application

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/thedataflows/nats-desktop/internal/models"
)

type BenchmarkManager struct {
	nc *nats.Conn
	js jetstream.JetStream
}

type benchmarkRunner struct {
	cancel    context.CancelFunc
	mu        sync.Mutex
	running   bool
	results   *models.BenchmarkResult
	latencies []time.Duration
}

func NewBenchmarkManager(nc *nats.Conn, js jetstream.JetStream) *BenchmarkManager {
	return &BenchmarkManager{
		nc: nc,
		js: js,
	}
}

func (bm *BenchmarkManager) RunPublishBenchmark(ctx context.Context, config *models.BenchmarkConfig) (*models.BenchmarkResult, error) {
	runner := &benchmarkRunner{
		results:   &models.BenchmarkResult{},
		latencies: []time.Duration{},
	}

	childCtx, cancel := context.WithCancel(ctx)

	var wg sync.WaitGroup

	payload := make([]byte, config.Size)
	if config.Payload != "" {
		payload = []byte(config.Payload)
	}

	messageCount := atomic.Uint64{}
	successCount := atomic.Uint64{}
	errorCount := atomic.Uint64{}
	bytesSent := atomic.Uint64{}

	startTime := time.Now()

	for i := 0; i < config.Clients; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			clientLatencies := make([]time.Duration, 0, config.Count/config.Clients)

			rate := config.MessageRate / config.Clients
			if rate <= 0 {
				rate = 1
			}
			ticker := time.NewTicker(time.Second / time.Duration(rate))
			defer ticker.Stop()

			for {
				select {
				case <-childCtx.Done():
					runner.mu.Lock()
					runner.latencies = append(runner.latencies, clientLatencies...)
					runner.mu.Unlock()
					return
				case <-ticker.C:
					start := time.Now()

					mCount := messageCount.Add(1)
					err := bm.nc.Publish(config.Subject, payload)
					if err != nil {
						errorCount.Add(1)
					} else {
						successCount.Add(1)
						bytesSent.Add(uint64(len(payload)))
						latency := time.Since(start)
						clientLatencies = append(clientLatencies, latency)
					}

					if config.OnProgress != nil {
						config.OnProgress(mCount, uint64(config.Count))
					}

					if config.Count > 0 && int(mCount) >= config.Count {
						runner.mu.Lock()
						runner.latencies = append(runner.latencies, clientLatencies...)
						runner.mu.Unlock()
						cancel()
						return
					}
				}
			}
		}()
	}

	wg.Wait()

	runner.mu.Lock()
	runner.results.TotalMessages = messageCount.Load()
	runner.results.SuccessCount = successCount.Load()
	runner.results.ErrorCount = errorCount.Load()
	runner.results.BytesSent = bytesSent.Load()
	runner.results.Duration = time.Since(startTime)
	if runner.results.Duration > 0 {
		runner.results.MessagesPerSec = float64(runner.results.TotalMessages) / runner.results.Duration.Seconds()
		runner.results.BytesPerSec = float64(runner.results.BytesSent) / runner.results.Duration.Seconds()
	}

	if len(runner.latencies) > 0 {
		runner.calculateLatencies()
	}
	runner.mu.Unlock()

	return runner.results, nil
}

func (bm *BenchmarkManager) RunSubscribeBenchmark(ctx context.Context, config *models.BenchmarkConfig) (*models.BenchmarkResult, error) {
	runner := &benchmarkRunner{
		results: &models.BenchmarkResult{},
	}

	childCtx, cancel := context.WithCancel(ctx)

	var wg sync.WaitGroup

	messageCount := atomic.Uint64{}
	bytesReceived := atomic.Uint64{}

	startTime := time.Now()

	for i := 0; i < config.Clients; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			var sub *nats.Subscription
			var err error

			msgChan := make(chan *nats.Msg, 1000)
			if config.QueueGroups {
				sub, err = bm.nc.ChanQueueSubscribe(config.Subject, "benchmark-group", msgChan)
			} else {
				sub, err = bm.nc.ChanSubscribe(config.Subject, msgChan)
			}

			if err != nil {
				return
			}
			defer sub.Unsubscribe()

			for {
				select {
				case <-childCtx.Done():
					return
				case msg := <-msgChan:
					bytesReceived.Add(uint64(len(msg.Data)))
					mCount := messageCount.Add(1)

					if config.OnProgress != nil {
						config.OnProgress(mCount, uint64(config.Count))
					}

					if config.Count > 0 && int(mCount) >= config.Count {
						cancel()
						return
					}
				}
			}
		}()
	}

	wg.Wait()

	runner.mu.Lock()
	runner.results.TotalMessages = messageCount.Load()
	runner.results.SuccessCount = messageCount.Load()
	runner.results.BytesReceived = bytesReceived.Load()
	runner.results.Duration = time.Since(startTime)
	if runner.results.Duration > 0 {
		runner.results.MessagesPerSec = float64(runner.results.TotalMessages) / runner.results.Duration.Seconds()
		runner.results.BytesPerSec = float64(runner.results.BytesReceived) / runner.results.Duration.Seconds()
	}
	runner.mu.Unlock()

	return runner.results, nil
}

func (bm *BenchmarkManager) RunRequestReplyBenchmark(ctx context.Context, config *models.BenchmarkConfig) (*models.BenchmarkResult, error) {
	runner := &benchmarkRunner{
		results:   &models.BenchmarkResult{},
		latencies: []time.Duration{},
	}

	childCtx, cancel := context.WithCancel(ctx)

	var wg sync.WaitGroup

	payload := make([]byte, config.Size)
	if config.Payload != "" {
		payload = []byte(config.Payload)
	}

	messageCount := atomic.Uint64{}
	successCount := atomic.Uint64{}
	errorCount := atomic.Uint64{}
	bytesSent := atomic.Uint64{}
	bytesReceived := atomic.Uint64{}

	startTime := time.Now()

	for i := 0; i < config.Clients; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			clientLatencies := make([]time.Duration, 0, config.Count/config.Clients)

			rate := config.MessageRate / config.Clients
			if rate <= 0 {
				rate = 1
			}
			ticker := time.NewTicker(time.Second / time.Duration(rate))
			defer ticker.Stop()

			for {
				select {
				case <-childCtx.Done():
					runner.mu.Lock()
					runner.latencies = append(runner.latencies, clientLatencies...)
					runner.mu.Unlock()
					return
				case <-ticker.C:
					start := time.Now()

					mCount := messageCount.Add(1)
					msg, err := bm.nc.RequestWithContext(childCtx, config.Subject, payload)
					if err != nil {
						errorCount.Add(1)
					} else {
						latency := time.Since(start)
						successCount.Add(1)
						bytesSent.Add(uint64(len(payload)))
						bytesReceived.Add(uint64(len(msg.Data)))
						clientLatencies = append(clientLatencies, latency)
					}

					if config.OnProgress != nil {
						config.OnProgress(mCount, uint64(config.Count))
					}

					if config.Count > 0 && int(mCount) >= config.Count {
						runner.mu.Lock()
						runner.latencies = append(runner.latencies, clientLatencies...)
						runner.mu.Unlock()
						cancel()
						return
					}
				}
			}
		}()
	}

	wg.Wait()

	runner.mu.Lock()
	runner.results.TotalMessages = messageCount.Load()
	runner.results.SuccessCount = successCount.Load()
	runner.results.ErrorCount = errorCount.Load()
	runner.results.BytesSent = bytesSent.Load()
	runner.results.BytesReceived = bytesReceived.Load()
	runner.results.Duration = time.Since(startTime)
	if runner.results.Duration > 0 {
		runner.results.MessagesPerSec = float64(runner.results.TotalMessages) / runner.results.Duration.Seconds()
		runner.results.BytesPerSec = float64(runner.results.BytesSent+runner.results.BytesReceived) / runner.results.Duration.Seconds()
	}

	if len(runner.latencies) > 0 {
		runner.calculateLatencies()
	}
	runner.mu.Unlock()

	return runner.results, nil
}

func (bm *BenchmarkManager) RunJetStreamBenchmark(ctx context.Context, config *models.BenchmarkConfig) (*models.BenchmarkResult, error) {
	if bm.js == nil {
		return nil, fmt.Errorf("jetstream not enabled")
	}

	runner := &benchmarkRunner{
		results:   &models.BenchmarkResult{},
		latencies: []time.Duration{},
	}

	childCtx, cancel := context.WithCancel(ctx)

	var wg sync.WaitGroup

	payload := make([]byte, config.Size)
	if config.Payload != "" {
		payload = []byte(config.Payload)
	}

	messageCount := atomic.Uint64{}
	successCount := atomic.Uint64{}
	errorCount := atomic.Uint64{}
	bytesSent := atomic.Uint64{}

	startTime := time.Now()

	for i := 0; i < config.Clients; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			clientLatencies := make([]time.Duration, 0, config.Count/config.Clients)

			rate := config.MessageRate / config.Clients
			if rate <= 0 {
				rate = 1
			}
			ticker := time.NewTicker(time.Second / time.Duration(rate))
			defer ticker.Stop()

			for {
				select {
				case <-childCtx.Done():
					runner.mu.Lock()
					runner.latencies = append(runner.latencies, clientLatencies...)
					runner.mu.Unlock()
					return
				case <-ticker.C:
					start := time.Now()

					mCount := messageCount.Add(1)
					_, err := bm.js.Publish(childCtx, config.Subject, payload)
					if err != nil {
						errorCount.Add(1)
					} else {
						latency := time.Since(start)
						successCount.Add(1)
						bytesSent.Add(uint64(len(payload)))
						clientLatencies = append(clientLatencies, latency)
					}

					if config.OnProgress != nil {
						config.OnProgress(mCount, uint64(config.Count))
					}

					if config.Count > 0 && int(mCount) >= config.Count {
						runner.mu.Lock()
						runner.latencies = append(runner.latencies, clientLatencies...)
						runner.mu.Unlock()
						cancel()
						return
					}
				}
			}
		}()
	}

	wg.Wait()

	runner.mu.Lock()
	runner.results.TotalMessages = messageCount.Load()
	runner.results.SuccessCount = successCount.Load()
	runner.results.ErrorCount = errorCount.Load()
	runner.results.BytesSent = bytesSent.Load()
	runner.results.Duration = time.Since(startTime)
	if runner.results.Duration > 0 {
		runner.results.MessagesPerSec = float64(runner.results.TotalMessages) / runner.results.Duration.Seconds()
		runner.results.BytesPerSec = float64(runner.results.BytesSent) / runner.results.Duration.Seconds()
	}

	if len(runner.latencies) > 0 {
		runner.calculateLatencies()
	}
	runner.mu.Unlock()

	return runner.results, nil
}

func (bm *BenchmarkManager) RunKVBenchmark(ctx context.Context, config *models.BenchmarkConfig) (*models.BenchmarkResult, error) {
	if bm.js == nil {
		return nil, fmt.Errorf("jetstream not enabled")
	}

	bucketName := fmt.Sprintf("BENCH_KV_%d", time.Now().Unix())

	kv, err := bm.js.CreateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket: bucketName,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create KV bucket: %w", err)
	}

	defer func() {
		deleteCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		bm.js.DeleteKeyValue(deleteCtx, bucketName)
	}()

	runner := &benchmarkRunner{
		results:   &models.BenchmarkResult{},
		latencies: []time.Duration{},
	}

	childCtx, cancel := context.WithCancel(ctx)

	var wg sync.WaitGroup

	payload := make([]byte, config.Size)
	if config.Payload != "" {
		payload = []byte(config.Payload)
	}

	putCount := atomic.Uint64{}
	getCount := atomic.Uint64{}
	successCount := atomic.Uint64{}
	errorCount := atomic.Uint64{}
	bytesSent := atomic.Uint64{}
	bytesReceived := atomic.Uint64{}

	startTime := time.Now()

	totalOps := config.Count
	if totalOps <= 0 {
		totalOps = 10000
	}

	for i := 0; i < config.Clients; i++ {
		wg.Add(1)
		go func(clientID int) {
			defer wg.Done()

			clientLatencies := make([]time.Duration, 0, totalOps/config.Clients)

			rate := config.MessageRate / config.Clients
			if rate <= 0 {
				rate = 1
			}
			ticker := time.NewTicker(time.Second / time.Duration(rate))
			defer ticker.Stop()

			for {
				select {
				case <-childCtx.Done():
					runner.mu.Lock()
					runner.latencies = append(runner.latencies, clientLatencies...)
					runner.mu.Unlock()
					return
				case <-ticker.C:
					key := fmt.Sprintf("key_%d_%d", clientID, putCount.Load())

					putStart := time.Now()
					_, err := kv.Put(childCtx, key, payload)
					if err != nil {
						errorCount.Add(1)
						continue
					}
					putLatency := time.Since(putStart)

					getStart := time.Now()
					entry, err := kv.Get(childCtx, key)
					if err != nil {
						errorCount.Add(1)
						continue
					}
					getLatency := time.Since(getStart)

					successCount.Add(2)
					putCount.Add(1)
					getCount.Add(1)
					bytesSent.Add(uint64(len(payload)))
					bytesReceived.Add(uint64(len(entry.Value())))

					clientLatencies = append(clientLatencies, putLatency, getLatency)

					totalCount := putCount.Load() + getCount.Load()
					if config.OnProgress != nil {
						config.OnProgress(totalCount, uint64(totalOps*2))
					}

					if totalOps > 0 && int(putCount.Load()) >= totalOps/config.Clients {
						runner.mu.Lock()
						runner.latencies = append(runner.latencies, clientLatencies...)
						runner.mu.Unlock()
						cancel()
						return
					}
				}
			}
		}(i)
	}

	wg.Wait()

	runner.mu.Lock()
	runner.results.TotalMessages = putCount.Load() + getCount.Load()
	runner.results.SuccessCount = successCount.Load()
	runner.results.ErrorCount = errorCount.Load()
	runner.results.BytesSent = bytesSent.Load()
	runner.results.BytesReceived = bytesReceived.Load()
	runner.results.Duration = time.Since(startTime)
	if runner.results.Duration > 0 {
		runner.results.MessagesPerSec = float64(runner.results.TotalMessages) / runner.results.Duration.Seconds()
		runner.results.BytesPerSec = float64(runner.results.BytesSent+runner.results.BytesReceived) / runner.results.Duration.Seconds()
	}

	if len(runner.latencies) > 0 {
		runner.calculateLatencies()
	}
	runner.mu.Unlock()

	return runner.results, nil
}

func (br *benchmarkRunner) calculateLatencies() {
	if len(br.latencies) == 0 {
		return
	}

	br.results.MinLatency = br.latencies[0]
	br.results.MaxLatency = br.latencies[0]

	total := time.Duration(0)
	for _, lat := range br.latencies {
		if lat < br.results.MinLatency {
			br.results.MinLatency = lat
		}
		if lat > br.results.MaxLatency {
			br.results.MaxLatency = lat
		}
		total += lat
	}
	br.results.AvgLatency = total / time.Duration(len(br.latencies))

	sorted := make([]time.Duration, len(br.latencies))
	copy(sorted, br.latencies)

	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i] < sorted[j]
	})

	br.results.P50Latency = sorted[len(sorted)*50/100]
	br.results.P95Latency = sorted[len(sorted)*95/100]
	br.results.P99Latency = sorted[len(sorted)*99/100]
}

func (br *benchmarkRunner) Stop() {
	if br.cancel != nil {
		br.cancel()
	}
	br.mu.Lock()
	br.running = false
	br.mu.Unlock()
}

func (br *benchmarkRunner) IsRunning() bool {
	br.mu.Lock()
	defer br.mu.Unlock()
	return br.running
}
