package nats

import (
	"context"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type Client struct {
	nc        *nats.Conn
	js        jetstream.JetStream
	connected bool
	url       string
}

type ConnectionConfig struct {
	URL      string
	Username string
	Password string
	Token    string
	NKeyFile string
	Creds    string
}

func NewClient(config ConnectionConfig) (*Client, error) {
	var opts []nats.Option

	if config.Username != "" && config.Password != "" {
		opts = append(opts, nats.UserInfo(config.Username, config.Password))
	}

	if config.Token != "" {
		opts = append(opts, nats.Token(config.Token))
	}

	if config.NKeyFile != "" {
		opts = append(opts, nats.Nkey(config.NKeyFile, nil))
	}

	if config.Creds != "" {
		opts = append(opts, nats.UserCredentials(config.Creds))
	}

	opts = append(opts,
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2*time.Second),
		nats.Timeout(2*time.Second),
	)

	nc, err := nats.Connect(config.URL, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("failed to create JetStream context: %w", err)
	}

	client := &Client{
		nc:        nc,
		js:        js,
		connected: true,
		url:       config.URL,
	}

	// Set up status change handlers after client is created
	nc.SetDisconnectErrHandler(func(nc *nats.Conn, err error) {
		client.connected = false
	})
	nc.SetReconnectHandler(func(nc *nats.Conn) {
		client.connected = true
	})
	nc.SetClosedHandler(func(nc *nats.Conn) {
		client.connected = false
	})

	return client, nil
}

func (c *Client) Close() error {
	if c.nc == nil {
		return nil
	}
	c.connected = false
	c.nc.Close()
	return nil
}

func (c *Client) IsConnected() bool {
	return c.connected && c.nc != nil && !c.nc.IsClosed()
}

func (c *Client) Conn() *nats.Conn {
	return c.nc
}

func (c *Client) JetStream() jetstream.JetStream {
	return c.js
}

func (c *Client) Publish(subject string, data []byte) error {
	if c.nc == nil {
		return fmt.Errorf("connection not established")
	}

	err := c.nc.Publish(subject, data)
	if err != nil {
		return fmt.Errorf("failed to publish: %w", err)
	}

	return nil
}

func (c *Client) PublishRequest(subject, reply string, data []byte) error {
	if c.nc == nil {
		return fmt.Errorf("connection not established")
	}

	err := c.nc.PublishRequest(subject, reply, data)
	if err != nil {
		return fmt.Errorf("failed to publish request: %w", err)
	}

	return nil
}

func (c *Client) Subscribe(subject string, handler nats.MsgHandler) (*nats.Subscription, error) {
	if c.nc == nil {
		return nil, fmt.Errorf("connection not established")
	}

	sub, err := c.nc.Subscribe(subject, handler)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe: %w", err)
	}

	return sub, nil
}

func (c *Client) SubscribeWithQueue(subject, queue string, handler nats.MsgHandler) (*nats.Subscription, error) {
	if c.nc == nil {
		return nil, fmt.Errorf("connection not established")
	}

	sub, err := c.nc.QueueSubscribe(subject, queue, handler)
	if err != nil {
		return nil, fmt.Errorf("failed to queue subscribe: %w", err)
	}

	return sub, nil
}

func (c *Client) Request(subject string, data []byte, timeout time.Duration) (*nats.Msg, error) {
	if c.nc == nil {
		return nil, fmt.Errorf("connection not established")
	}

	msg, err := c.nc.Request(subject, data, timeout)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	return msg, nil
}

func (c *Client) RequestWithContext(ctx context.Context, subject string, data []byte) (*nats.Msg, error) {
	if c.nc == nil {
		return nil, fmt.Errorf("connection not established")
	}

	return c.nc.RequestWithContext(ctx, subject, data)
}

func (c *Client) GetStatus() string {
	if !c.IsConnected() {
		return "Disconnected"
	}
	return "Connected"
}

func (c *Client) Servers() []string {
	if c.nc == nil {
		return nil
	}
	return c.nc.Servers()
}

func (c *Client) ConnectedServerID() string {
	if c.nc == nil {
		return ""
	}
	return c.nc.ConnectedServerId()
}

func (c *Client) ConnectedServerName() string {
	if c.nc == nil {
		return ""
	}
	return c.nc.ConnectedServerName()
}

func (c *Client) ConnectedServerVersion() string {
	if c.nc == nil {
		return ""
	}
	return c.nc.ConnectedServerVersion()
}

func (c *Client) ConnectedAddr() string {
	if c.nc == nil {
		return ""
	}
	return c.nc.ConnectedAddr()
}

func (c *Client) RTT() (time.Duration, error) {
	if c.nc == nil {
		return 0, fmt.Errorf("not connected")
	}
	return c.nc.RTT()
}

func (c *Client) DiscoveredServers() []string {
	if c.nc == nil {
		return nil
	}
	return c.nc.DiscoveredServers()
}

func (c *Client) GetJetStream() jetstream.JetStream {
	return c.js
}

func (c *Client) GetAccountInfo(ctx context.Context) (*jetstream.AccountInfo, error) {
	if !c.IsConnected() {
		return nil, fmt.Errorf("not connected")
	}

	if c.js == nil {
		return nil, fmt.Errorf("JetStream not initialized")
	}

	return c.js.AccountInfo(ctx)
}

func (c *Client) ListStreams(ctx context.Context) ([]jetstream.StreamInfo, error) {
	if !c.IsConnected() {
		return nil, fmt.Errorf("not connected")
	}

	if c.js == nil {
		return nil, fmt.Errorf("JetStream not initialized")
	}

	iterator := c.js.ListStreams(ctx)

	var streams []jetstream.StreamInfo
	for info := range iterator.Info() {
		if info != nil {
			streams = append(streams, *info)
		}
	}

	if iterator.Err() != nil {
		return nil, fmt.Errorf("error iterating streams: %w", iterator.Err())
	}

	return streams, nil
}

func (c *Client) GetStream(ctx context.Context, name string) (jetstream.Stream, error) {
	if !c.IsConnected() || c.js == nil {
		return nil, fmt.Errorf("not connected")
	}
	return c.js.Stream(ctx, name)
}

func (c *Client) GetStreamMessages(ctx context.Context, stream string, count int, startSeq uint64) ([]*jetstream.RawStreamMsg, int64, error) {
	if !c.IsConnected() || c.js == nil {
		return nil, 0, fmt.Errorf("not connected")
	}

	s, err := c.js.Stream(ctx, stream)
	if err != nil {
		return nil, 0, err
	}

	info := s.CachedInfo()
	total := int64(info.State.Msgs)

	var messages []*jetstream.RawStreamMsg

	// If startSeq is 0, start from the last message
	currSeq := startSeq
	if currSeq == 0 {
		currSeq = info.State.LastSeq
	}

	// Fetch up to 'count' messages starting from currSeq
	// We handle holes by skipping up to a limit
	for len(messages) < count && currSeq > 0 {
		msg, err := s.GetMsg(ctx, currSeq)
		if err == nil && msg != nil {
			messages = append(messages, msg)
		}
		if currSeq == 1 {
			break
		}
		currSeq--
	}

	return messages, total, nil
}

func (c *Client) CreateStream(ctx context.Context, config jetstream.StreamConfig) (jetstream.Stream, error) {
	if !c.IsConnected() {
		return nil, fmt.Errorf("not connected")
	}

	if c.js == nil {
		return nil, fmt.Errorf("JetStream not initialized")
	}

	stream, err := c.js.CreateStream(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create stream: %w", err)
	}

	return stream, nil
}

func (c *Client) UpdateStream(ctx context.Context, config jetstream.StreamConfig) (jetstream.Stream, error) {
	if !c.IsConnected() {
		return nil, fmt.Errorf("not connected")
	}

	if c.js == nil {
		return nil, fmt.Errorf("JetStream not initialized")
	}

	stream, err := c.js.UpdateStream(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to update stream: %w", err)
	}

	return stream, nil
}

func (c *Client) DeleteStream(ctx context.Context, name string) error {
	if !c.IsConnected() {
		return fmt.Errorf("not connected")
	}

	if c.js == nil {
		return fmt.Errorf("JetStream not initialized")
	}

	err := c.js.DeleteStream(ctx, name)
	if err != nil {
		return fmt.Errorf("failed to delete stream: %w", err)
	}

	return nil
}

func (c *Client) PurgeStream(ctx context.Context, name string) error {
	if !c.IsConnected() {
		return fmt.Errorf("not connected")
	}

	if c.js == nil {
		return fmt.Errorf("JetStream not initialized")
	}

	stream, err := c.js.Stream(ctx, name)
	if err != nil {
		return fmt.Errorf("failed to get stream: %w", err)
	}

	err = stream.Purge(ctx)
	if err != nil {
		return fmt.Errorf("failed to purge stream: %w", err)
	}

	return nil
}

func (c *Client) DeleteStreamMessage(ctx context.Context, streamName string, seq uint64) error {
	if !c.IsConnected() {
		return fmt.Errorf("not connected")
	}

	if c.js == nil {
		return fmt.Errorf("JetStream not initialized")
	}

	stream, err := c.js.Stream(ctx, streamName)
	if err != nil {
		return fmt.Errorf("failed to get stream: %w", err)
	}

	err = stream.DeleteMsg(ctx, seq)
	if err != nil {
		return fmt.Errorf("failed to delete message: %w", err)
	}

	return nil
}

func (c *Client) ListKeyValueStores(ctx context.Context) ([]jetstream.KeyValueStatus, error) {
	if !c.IsConnected() {
		return nil, fmt.Errorf("not connected")
	}

	if c.js == nil {
		return nil, fmt.Errorf("JetStream not initialized")
	}

	iterator := c.js.KeyValueStoreNames(ctx)
	var stores []jetstream.KeyValueStatus
	for name := range iterator.Name() {
		kv, err := c.js.KeyValue(ctx, name)
		if err != nil {
			continue
		}
		status, err := kv.Status(ctx)
		if err != nil {
			continue
		}
		stores = append(stores, status)
	}

	return stores, nil
}

func (c *Client) ListObjectStores(ctx context.Context) ([]jetstream.ObjectStoreStatus, error) {
	if !c.IsConnected() {
		return nil, fmt.Errorf("not connected")
	}

	if c.js == nil {
		return nil, fmt.Errorf("JetStream not initialized")
	}

	iterator := c.js.ObjectStoreNames(ctx)
	var stores []jetstream.ObjectStoreStatus
	for name := range iterator.Name() {
		obs, err := c.js.ObjectStore(ctx, name)
		if err != nil {
			continue
		}
		status, err := obs.Status(ctx)
		if err != nil {
			continue
		}
		stores = append(stores, status)
	}

	return stores, nil
}

func (c *Client) DeleteConsumer(ctx context.Context, stream, consumer string) error {
	if !c.IsConnected() || c.js == nil {
		return fmt.Errorf("not connected")
	}
	return c.js.DeleteConsumer(ctx, stream, consumer)
}

func (c *Client) CreateConsumer(ctx context.Context, stream, consumer string) (jetstream.Consumer, error) {
	if !c.IsConnected() || c.js == nil {
		return nil, fmt.Errorf("not connected")
	}
	return c.js.CreateConsumer(ctx, stream, jetstream.ConsumerConfig{
		Durable: consumer,
	})
}

func (c *Client) CreateConsumerWithConfig(ctx context.Context, stream string, config jetstream.ConsumerConfig) (jetstream.Consumer, error) {
	if !c.IsConnected() || c.js == nil {
		return nil, fmt.Errorf("not connected")
	}
	return c.js.CreateConsumer(ctx, stream, config)
}

func (c *Client) PauseConsumer(ctx context.Context, stream, consumer string, pause time.Duration) error {
	if !c.IsConnected() || c.js == nil {
		return fmt.Errorf("not connected")
	}
	_, err := c.js.PauseConsumer(ctx, stream, consumer, time.Now().Add(pause))
	return err
}

func (c *Client) ResumeConsumer(ctx context.Context, stream, consumer string) error {
	if !c.IsConnected() || c.js == nil {
		return fmt.Errorf("not connected")
	}
	_, err := c.js.ResumeConsumer(ctx, stream, consumer)
	return err
}

func (c *Client) CreateKeyValue(ctx context.Context, config jetstream.KeyValueConfig) (jetstream.KeyValue, error) {
	if !c.IsConnected() || c.js == nil {
		return nil, fmt.Errorf("not connected")
	}
	return c.js.CreateKeyValue(ctx, config)
}

func (c *Client) DeleteKeyValue(ctx context.Context, bucket string) error {
	if !c.IsConnected() || c.js == nil {
		return fmt.Errorf("not connected")
	}
	return c.js.DeleteKeyValue(ctx, bucket)
}

func (c *Client) PurgeKeyValue(ctx context.Context, bucket string) error {
	if !c.IsConnected() || c.js == nil {
		return fmt.Errorf("not connected")
	}
	streamName := fmt.Sprintf("KV_%s", bucket)
	stream, err := c.js.Stream(ctx, streamName)
	if err != nil {
		return fmt.Errorf("failed to get underlying stream for bucket %s: %w", bucket, err)
	}
	return stream.Purge(ctx)
}

func (c *Client) ListKeyValueKeys(ctx context.Context, bucket string) ([]string, error) {
	if !c.IsConnected() || c.js == nil {
		return nil, fmt.Errorf("not connected")
	}
	kv, err := c.js.KeyValue(ctx, bucket)
	if err != nil {
		return nil, err
	}
	lister, err := kv.ListKeys(ctx)
	if err != nil {
		return nil, err
	}
	defer lister.Stop()

	var keys []string
	for key := range lister.Keys() {
		keys = append(keys, key)
	}
	return keys, nil
}

func (c *Client) ListKeyValueEntries(ctx context.Context, bucket string) ([]jetstream.KeyValueEntry, error) {
	if !c.IsConnected() || c.js == nil {
		return nil, fmt.Errorf("not connected")
	}
	kv, err := c.js.KeyValue(ctx, bucket)
	if err != nil {
		return nil, err
	}

	// Get stream info to know the last sequence
	stream, err := c.js.Stream(ctx, fmt.Sprintf("KV_%s", bucket))
	if err != nil {
		return nil, err
	}
	info, err := stream.Info(ctx)
	if err != nil {
		return nil, err
	}

	lastSeq := info.State.LastSeq
	if lastSeq == 0 {
		return nil, nil
	}

	watcher, err := kv.Watch(ctx, ">", jetstream.IncludeHistory())
	if err != nil {
		return nil, err
	}
	defer watcher.Stop()

	// Use a map to keep only the latest entry for each key
	latestEntries := make(map[string]jetstream.KeyValueEntry)
	for entry := range watcher.Updates() {
		if entry == nil {
			// nats.go sends nil to indicate catching up with history is complete.
			break
		}
		latestEntries[entry.Key()] = entry
		if entry.Delta() == 0 {
			// Fallback: also break if we explicitly see delta 0.
			break
		}
	}

	entries := make([]jetstream.KeyValueEntry, 0, len(latestEntries))
	for _, entry := range latestEntries {
		entries = append(entries, entry)
	}
	return entries, nil
}

func (c *Client) GetKeyValue(ctx context.Context, bucket, key string) ([]byte, error) {
	if !c.IsConnected() || c.js == nil {
		return nil, fmt.Errorf("not connected")
	}
	kv, err := c.js.KeyValue(ctx, bucket)
	if err != nil {
		return nil, err
	}
	entry, err := kv.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	return entry.Value(), nil
}

func (c *Client) GetKeyValueStreamInfo(ctx context.Context, bucket string) (*jetstream.StreamInfo, error) {
	if !c.IsConnected() || c.js == nil {
		return nil, fmt.Errorf("not connected")
	}
	stream, err := c.js.Stream(ctx, fmt.Sprintf("KV_%s", bucket))
	if err != nil {
		return nil, err
	}
	return stream.Info(ctx)
}

func (c *Client) DeleteKeyValueKey(ctx context.Context, bucket, key string) error {
	if !c.IsConnected() || c.js == nil {
		return fmt.Errorf("not connected")
	}
	kv, err := c.js.KeyValue(ctx, bucket)
	if err != nil {
		return err
	}
	return kv.Delete(ctx, key)
}

func (c *Client) PurgeKeyValueKey(ctx context.Context, bucket, key string) error {
	if !c.IsConnected() || c.js == nil {
		return fmt.Errorf("not connected")
	}

	streamName := fmt.Sprintf("KV_%s", bucket)
	subject := fmt.Sprintf("$KV.%s.%s", bucket, key)

	stream, err := c.js.Stream(ctx, streamName)
	if err != nil {
		return fmt.Errorf("failed to get underlying stream for bucket %s: %w", bucket, err)
	}

	// Physically remove all messages for this subject (key) from the stream.
	// This removes the key entirely from NATS, including any history and tombstones.
	return stream.Purge(ctx, jetstream.WithPurgeSubject(subject))
}

func (c *Client) CreateObjectStore(ctx context.Context, config jetstream.ObjectStoreConfig) (jetstream.ObjectStore, error) {
	if !c.IsConnected() || c.js == nil {
		return nil, fmt.Errorf("not connected")
	}
	return c.js.CreateObjectStore(ctx, config)
}

func (c *Client) DeleteObjectStore(ctx context.Context, bucket string) error {
	if !c.IsConnected() || c.js == nil {
		return fmt.Errorf("not connected")
	}
	return c.js.DeleteObjectStore(ctx, bucket)
}

func (c *Client) ListServices(ctx context.Context) ([]*nats.Msg, error) {
	if !c.IsConnected() {
		return nil, fmt.Errorf("not connected")
	}

	// This is a simplified service discovery.
	// In a real scenario, we would use the micro package or send PING to $SRV.PING
	// and collect responses over a short period.
	sub, err := c.nc.SubscribeSync(nats.NewInbox())
	if err != nil {
		return nil, err
	}
	defer sub.Unsubscribe()

	err = c.nc.PublishRequest("$SRV.PING", sub.Subject, nil)
	if err != nil {
		return nil, err
	}

	var responses []*nats.Msg
	for {
		msg, err := sub.NextMsgWithContext(ctx)
		if err != nil {
			break
		}
		responses = append(responses, msg)
	}

	return responses, nil
}

// MessageGap represents a gap in message sequences
type MessageGap struct {
	StartSeq uint64
	EndSeq   uint64
	Count    uint64
}

// CopyStream creates a new stream with the same configuration as an existing stream
func (c *Client) CopyStream(ctx context.Context, sourceStreamName, newStreamName string) (jetstream.Stream, error) {
	if !c.IsConnected() {
		return nil, fmt.Errorf("not connected")
	}

	if c.js == nil {
		return nil, fmt.Errorf("JetStream not initialized")
	}

	sourceStream, err := c.js.Stream(ctx, sourceStreamName)
	if err != nil {
		return nil, fmt.Errorf("failed to get source stream: %w", err)
	}

	info, err := sourceStream.Info(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get stream info: %w", err)
	}

	config := info.Config
	config.Name = newStreamName

	newStream, err := c.js.CreateStream(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create stream copy: %w", err)
	}

	return newStream, nil
}

// SealStream seals a stream to prevent further updates
func (c *Client) SealStream(ctx context.Context, streamName string) error {
	if !c.IsConnected() {
		return fmt.Errorf("not connected")
	}

	if c.js == nil {
		return fmt.Errorf("JetStream not initialized")
	}

	stream, err := c.js.Stream(ctx, streamName)
	if err != nil {
		return fmt.Errorf("failed to get stream: %w", err)
	}

	info, err := stream.Info(ctx)
	if err != nil {
		return fmt.Errorf("failed to get stream info: %w", err)
	}

	config := info.Config
	config.Sealed = true

	_, err = c.js.UpdateStream(ctx, config)
	if err != nil {
		return fmt.Errorf("failed to seal stream: %w", err)
	}

	return nil
}

// GetStreamMessageBySequence retrieves a single message by its sequence number
func (c *Client) GetStreamMessageBySequence(ctx context.Context, streamName string, seq uint64) (*jetstream.RawStreamMsg, error) {
	if !c.IsConnected() {
		return nil, fmt.Errorf("not connected")
	}

	if c.js == nil {
		return nil, fmt.Errorf("JetStream not initialized")
	}

	stream, err := c.js.Stream(ctx, streamName)
	if err != nil {
		return nil, fmt.Errorf("failed to get stream: %w", err)
	}

	msg, err := stream.GetMsg(ctx, seq)
	if err != nil {
		return nil, fmt.Errorf("failed to get message: %w", err)
	}

	return msg, nil
}

// GetStreamMessageBySubject retrieves the last message for a given subject in a stream
func (c *Client) GetStreamMessageBySubject(ctx context.Context, streamName string, subject string) (*jetstream.RawStreamMsg, error) {
	if !c.IsConnected() {
		return nil, fmt.Errorf("not connected")
	}

	if c.js == nil {
		return nil, fmt.Errorf("JetStream not initialized")
	}

	stream, err := c.js.Stream(ctx, streamName)
	if err != nil {
		return nil, fmt.Errorf("failed to get stream: %w", err)
	}

	msg, err := stream.GetLastMsgForSubject(ctx, subject)
	if err != nil {
		return nil, fmt.Errorf("failed to get message for subject: %w", err)
	}

	return msg, nil
}

// UpdateConsumer updates an existing consumer's configuration
func (c *Client) UpdateConsumer(ctx context.Context, streamName string, config jetstream.ConsumerConfig) (jetstream.Consumer, error) {
	if !c.IsConnected() {
		return nil, fmt.Errorf("not connected")
	}

	if c.js == nil {
		return nil, fmt.Errorf("JetStream not initialized")
	}

	stream, err := c.js.Stream(ctx, streamName)
	if err != nil {
		return nil, fmt.Errorf("failed to get stream: %w", err)
	}

	consumer, err := stream.UpdateConsumer(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to update consumer: %w", err)
	}

	return consumer, nil
}

// CopyConsumer creates a new consumer with the same configuration as an existing one
func (c *Client) CopyConsumer(ctx context.Context, streamName, sourceConsumerName, newConsumerName string) (jetstream.Consumer, error) {
	if !c.IsConnected() {
		return nil, fmt.Errorf("not connected")
	}

	if c.js == nil {
		return nil, fmt.Errorf("JetStream not initialized")
	}

	stream, err := c.js.Stream(ctx, streamName)
	if err != nil {
		return nil, fmt.Errorf("failed to get stream: %w", err)
	}

	sourceConsumer, err := stream.Consumer(ctx, sourceConsumerName)
	if err != nil {
		return nil, fmt.Errorf("failed to get source consumer: %w", err)
	}

	info, err := sourceConsumer.Info(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get consumer info: %w", err)
	}

	config := info.Config
	config.Name = newConsumerName
	config.Durable = newConsumerName

	newConsumer, err := stream.CreateConsumer(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create consumer copy: %w", err)
	}

	return newConsumer, nil
}

// WatchKeyValue watches a KV bucket for changes
func (c *Client) WatchKeyValue(ctx context.Context, bucket string, key string) (jetstream.KeyWatcher, error) {
	if !c.IsConnected() || c.js == nil {
		return nil, fmt.Errorf("not connected")
	}

	kv, err := c.js.KeyValue(ctx, bucket)
	if err != nil {
		return nil, fmt.Errorf("failed to get KV bucket: %w", err)
	}

	watcher, err := kv.Watch(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("failed to start watcher: %w", err)
	}

	return watcher, nil
}

// GetKeyValueHistory retrieves the history of a key
func (c *Client) GetKeyValueHistory(ctx context.Context, bucket string, key string) ([]jetstream.KeyValueEntry, error) {
	if !c.IsConnected() || c.js == nil {
		return nil, fmt.Errorf("not connected")
	}

	kv, err := c.js.KeyValue(ctx, bucket)
	if err != nil {
		return nil, fmt.Errorf("failed to get KV bucket: %w", err)
	}

	history, err := kv.History(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("failed to get key history: %w", err)
	}

	return history, nil
}

// UpdateKeyValue updates a key with revision check (CAS operation)
func (c *Client) UpdateKeyValue(ctx context.Context, bucket string, key string, value []byte, revision uint64) error {
	if !c.IsConnected() || c.js == nil {
		return fmt.Errorf("not connected")
	}

	kv, err := c.js.KeyValue(ctx, bucket)
	if err != nil {
		return fmt.Errorf("failed to get KV bucket: %w", err)
	}

	_, err = kv.Update(ctx, key, value, revision)
	if err != nil {
		return fmt.Errorf("failed to update key: %w", err)
	}

	return nil
}

// CompactKeyValue compacts a KV bucket
func (c *Client) CompactKeyValue(ctx context.Context, bucket string) error {
	if !c.IsConnected() || c.js == nil {
		return fmt.Errorf("not connected")
	}

	kv, err := c.js.KeyValue(ctx, bucket)
	if err != nil {
		return fmt.Errorf("failed to get KV bucket: %w", err)
	}

	keys, err := c.ListKeyValueKeys(ctx, bucket)
	if err != nil {
		return fmt.Errorf("failed to list keys: %w", err)
	}

	for _, key := range keys {
		if err := kv.Purge(ctx, key); err != nil {
			continue
		}
	}

	return nil
}

// PutObject puts an object into the object store
func (c *Client) PutObject(ctx context.Context, bucket string, objectName string, data []byte) (*jetstream.ObjectInfo, error) {
	if !c.IsConnected() {
		return nil, fmt.Errorf("not connected")
	}

	if c.js == nil {
		return nil, fmt.Errorf("JetStream not initialized")
	}

	obs, err := c.js.ObjectStore(ctx, bucket)
	if err != nil {
		return nil, fmt.Errorf("failed to get object store: %w", err)
	}

	info, err := obs.PutBytes(ctx, objectName, data)
	if err != nil {
		return nil, fmt.Errorf("failed to put object: %w", err)
	}

	return info, nil
}

// GetObject retrieves an object from the object store
func (c *Client) GetObject(ctx context.Context, bucket string, objectName string) ([]byte, error) {
	if !c.IsConnected() {
		return nil, fmt.Errorf("not connected")
	}

	if c.js == nil {
		return nil, fmt.Errorf("JetStream not initialized")
	}

	obs, err := c.js.ObjectStore(ctx, bucket)
	if err != nil {
		return nil, fmt.Errorf("failed to get object store: %w", err)
	}

	data, err := obs.GetBytes(ctx, objectName)
	if err != nil {
		return nil, fmt.Errorf("failed to get object: %w", err)
	}

	return data, nil
}

// DeleteObject deletes an object from the object store
func (c *Client) DeleteObject(ctx context.Context, bucket string, objectName string) error {
	if !c.IsConnected() {
		return fmt.Errorf("not connected")
	}

	if c.js == nil {
		return fmt.Errorf("JetStream not initialized")
	}

	obs, err := c.js.ObjectStore(ctx, bucket)
	if err != nil {
		return fmt.Errorf("failed to get object store: %w", err)
	}

	err = obs.Delete(ctx, objectName)
	if err != nil {
		return fmt.Errorf("failed to delete object: %w", err)
	}

	return nil
}

// GetObjectInfo retrieves information about an object
func (c *Client) GetObjectInfo(ctx context.Context, bucket string, objectName string) (*jetstream.ObjectInfo, error) {
	if !c.IsConnected() {
		return nil, fmt.Errorf("not connected")
	}

	if c.js == nil {
		return nil, fmt.Errorf("JetStream not initialized")
	}

	obs, err := c.js.ObjectStore(ctx, bucket)
	if err != nil {
		return nil, fmt.Errorf("failed to get object store: %w", err)
	}

	info, err := obs.GetInfo(ctx, objectName)
	if err != nil {
		return nil, fmt.Errorf("failed to get object info: %w", err)
	}

	return info, nil
}

// ListObjects lists all objects in a bucket
func (c *Client) ListObjects(ctx context.Context, bucket string) ([]*jetstream.ObjectInfo, error) {
	if !c.IsConnected() {
		return nil, fmt.Errorf("not connected")
	}

	if c.js == nil {
		return nil, fmt.Errorf("JetStream not initialized")
	}

	obs, err := c.js.ObjectStore(ctx, bucket)
	if err != nil {
		return nil, fmt.Errorf("failed to get object store: %w", err)
	}

	listCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	objects, err := obs.List(listCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to list objects: %w", err)
	}

	return objects, nil
}

// GetObjectStoreStreamInfo retrieves the underlying stream info for an object store
func (c *Client) GetObjectStoreStreamInfo(ctx context.Context, bucket string) (*jetstream.StreamInfo, error) {
	if !c.IsConnected() || c.js == nil {
		return nil, fmt.Errorf("not connected")
	}
	stream, err := c.js.Stream(ctx, fmt.Sprintf("OBJ_%s", bucket))
	if err != nil {
		return nil, err
	}
	return stream.Info(ctx)
}

// UpdateKeyValueStore updates an existing KV bucket configuration
func (c *Client) UpdateKeyValueStore(ctx context.Context, bucket string, config jetstream.KeyValueConfig) error {
	if !c.IsConnected() {
		return fmt.Errorf("not connected")
	}

	if c.js == nil {
		return fmt.Errorf("JetStream not initialized")
	}

	// KV buckets are backed by streams with naming convention "KV_<bucket>"
	streamName := fmt.Sprintf("KV_%s", bucket)
	stream, err := c.js.Stream(ctx, streamName)
	if err != nil {
		return fmt.Errorf("failed to get KV stream: %w", err)
	}

	info, err := stream.Info(ctx)
	if err != nil {
		return fmt.Errorf("failed to get stream info: %w", err)
	}

	// Update the stream configuration with new KV settings
	streamConfig := info.Config

	// Update max bytes if specified
	if config.MaxBytes > 0 {
		streamConfig.MaxBytes = config.MaxBytes
	}

	// Update max message size (for max value size)
	if config.MaxValueSize > 0 {
		streamConfig.MaxMsgSize = int32(config.MaxValueSize)
	}

	// Update TTL
	if config.TTL > 0 {
		streamConfig.MaxAge = config.TTL
	}

	// Update description
	if config.Description != "" {
		streamConfig.Description = config.Description
	}

	_, err = c.js.UpdateStream(ctx, streamConfig)
	if err != nil {
		return fmt.Errorf("failed to update KV bucket: %w", err)
	}

	return nil
}

// DetectMessageGaps scans a stream and returns any sequence gaps
func (c *Client) DetectMessageGaps(ctx context.Context, streamName string) ([]MessageGap, error) {
	if !c.IsConnected() {
		return nil, fmt.Errorf("not connected")
	}

	if c.js == nil {
		return nil, fmt.Errorf("JetStream not initialized")
	}

	stream, err := c.js.Stream(ctx, streamName)
	if err != nil {
		return nil, fmt.Errorf("failed to get stream: %w", err)
	}

	info, err := stream.Info(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get stream info: %w", err)
	}

	var gaps []MessageGap
	lastSeq := info.State.LastSeq
	firstSeq := info.State.FirstSeq

	if lastSeq == 0 || firstSeq > lastSeq {
		return gaps, nil
	}

	// Scan for gaps by checking every sequence
	var prevSeq uint64
	for seq := firstSeq; seq <= lastSeq; seq++ {
		msg, err := stream.GetMsg(ctx, seq)
		if err != nil || msg == nil {
			// This sequence doesn't exist - it's a gap
			if prevSeq == 0 {
				prevSeq = seq
			}
		} else {
			// Message exists - check if we were tracking a gap
			if prevSeq != 0 {
				gaps = append(gaps, MessageGap{
					StartSeq: prevSeq,
					EndSeq:   seq - 1,
					Count:    seq - prevSeq,
				})
				prevSeq = 0
			}
		}
	}

	// Handle gap at the end
	if prevSeq != 0 && prevSeq <= lastSeq {
		gaps = append(gaps, MessageGap{
			StartSeq: prevSeq,
			EndSeq:   lastSeq,
			Count:    lastSeq - prevSeq + 1,
		})
	}

	return gaps, nil
}

// StreamBackup represents a complete backup of a stream
type StreamBackup struct {
	Config    jetstream.StreamConfig `json:"config"`
	State     jetstream.StreamState  `json:"state"`
	Messages  []BackupMessage        `json:"messages"`
	Consumers []ConsumerBackup       `json:"consumers"`
}

// BackupMessage represents a message in the backup
type BackupMessage struct {
	Sequence uint64            `json:"seq"`
	Subject  string            `json:"subject"`
	Data     []byte            `json:"data"`
	Headers  map[string]string `json:"headers,omitempty"`
	Time     time.Time         `json:"time"`
}

// ConsumerBackup represents a consumer's backup data
type ConsumerBackup struct {
	Name   string                   `json:"name"`
	Config jetstream.ConsumerConfig `json:"config"`
}

// BackupStream creates a complete backup of a stream including all messages and consumers
func (c *Client) BackupStream(ctx context.Context, streamName string) (*StreamBackup, error) {
	if !c.IsConnected() {
		return nil, fmt.Errorf("not connected")
	}

	if c.js == nil {
		return nil, fmt.Errorf("JetStream not initialized")
	}

	stream, err := c.js.Stream(ctx, streamName)
	if err != nil {
		return nil, fmt.Errorf("failed to get stream: %w", err)
	}

	// Get stream info
	info, err := stream.Info(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get stream info: %w", err)
	}

	backup := &StreamBackup{
		Config: info.Config,
		State:  info.State,
	}

	// Backup all messages
	if info.State.LastSeq > 0 {
		backup.Messages = make([]BackupMessage, 0)
		for seq := info.State.FirstSeq; seq <= info.State.LastSeq; seq++ {
			msg, err := stream.GetMsg(ctx, seq)
			if err != nil {
				// Message may have been deleted or expired, skip it
				continue
			}

			// Convert headers
			headers := make(map[string]string)
			if msg.Header != nil {
				for k, v := range msg.Header {
					if len(v) > 0 {
						headers[k] = v[0]
					}
				}
			}

			backup.Messages = append(backup.Messages, BackupMessage{
				Sequence: seq,
				Subject:  msg.Subject,
				Data:     msg.Data,
				Headers:  headers,
				Time:     msg.Time,
			})
		}
	}

	// Backup consumers using stream's ConsumerManager
	consumerLister := stream.ListConsumers(ctx)
	backup.Consumers = make([]ConsumerBackup, 0)
	for info := range consumerLister.Info() {
		if info != nil {
			backup.Consumers = append(backup.Consumers, ConsumerBackup{
				Name:   info.Name,
				Config: info.Config,
			})
		}
	}

	return backup, nil
}
