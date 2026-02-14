/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package streaming

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/IBM/sarama"
)

// Error messages used by the Kafka publisher.
const (
	errMsgPublisherClosed = "publisher is closed"
	errMsgNilEvent        = "event must not be nil"
	errMsgMarshalFailed   = "failed to marshal event"
)

// saramaProducer abstracts the sarama.AsyncProducer for testing.
type saramaProducer interface {
	Input() chan<- *sarama.ProducerMessage
	Errors() <-chan *sarama.ProducerError
	AsyncClose()
	Close() error
}

// KafkaPublisher publishes session events to Kafka using an async producer.
type KafkaPublisher struct {
	producer saramaProducer
	topic    string
	strategy PartitionStrategy
	logger   *slog.Logger

	mu     sync.RWMutex
	closed bool
	wg     sync.WaitGroup
}

// NewKafkaPublisher creates a KafkaPublisher with the given config.
func NewKafkaPublisher(cfg *KafkaConfig, logger *slog.Logger) (*KafkaPublisher, error) {
	saramaCfg, err := buildSaramaConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("invalid kafka config: %w", err)
	}

	producer, err := sarama.NewAsyncProducer(cfg.Brokers, saramaCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create kafka producer: %w", err)
	}

	return newKafkaPublisherWithProducer(producer, cfg.Topic, cfg.PartitionStrategy, logger), nil
}

// newKafkaPublisherWithProducer creates a KafkaPublisher with an injected producer (for testing).
func newKafkaPublisherWithProducer(
	producer saramaProducer,
	topic string,
	strategy PartitionStrategy,
	logger *slog.Logger,
) *KafkaPublisher {
	if logger == nil {
		logger = slog.Default()
	}

	kp := &KafkaPublisher{
		producer: producer,
		topic:    topic,
		strategy: strategy,
		logger:   logger,
	}

	kp.wg.Add(1)
	go kp.drainErrors()

	return kp
}

// Publish sends a single session event to Kafka. It is non-blocking.
func (kp *KafkaPublisher) Publish(_ context.Context, event *SessionEvent) error {
	if event == nil {
		return errors.New(errMsgNilEvent)
	}

	kp.mu.RLock()
	if kp.closed {
		kp.mu.RUnlock()
		return errors.New(errMsgPublisherClosed)
	}
	kp.mu.RUnlock()

	msg, err := kp.buildMessage(event)
	if err != nil {
		return err
	}

	kp.producer.Input() <- msg

	return nil
}

// PublishBatch sends multiple session events to Kafka. It is non-blocking.
func (kp *KafkaPublisher) PublishBatch(_ context.Context, events []*SessionEvent) error {
	kp.mu.RLock()
	if kp.closed {
		kp.mu.RUnlock()
		return errors.New(errMsgPublisherClosed)
	}
	kp.mu.RUnlock()

	for _, event := range events {
		if event == nil {
			continue
		}

		msg, err := kp.buildMessage(event)
		if err != nil {
			return err
		}

		kp.producer.Input() <- msg
	}

	return nil
}

// Close shuts down the producer and waits for the error drain goroutine.
func (kp *KafkaPublisher) Close() error {
	kp.mu.Lock()
	if kp.closed {
		kp.mu.Unlock()
		return nil
	}
	kp.closed = true
	kp.mu.Unlock()

	kp.producer.AsyncClose()
	kp.wg.Wait()

	return nil
}

// buildMessage creates a sarama ProducerMessage from a SessionEvent.
func (kp *KafkaPublisher) buildMessage(event *SessionEvent) (*sarama.ProducerMessage, error) {
	data, err := json.Marshal(event)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", errMsgMarshalFailed, err)
	}

	msg := &sarama.ProducerMessage{
		Topic: kp.topic,
		Value: sarama.ByteEncoder(data),
	}

	msg.Key = kp.partitionKey(event)

	return msg, nil
}

// partitionKey returns the appropriate key encoder based on the partition strategy.
func (kp *KafkaPublisher) partitionKey(event *SessionEvent) sarama.Encoder {
	switch kp.strategy {
	case PartitionBySessionID:
		return sarama.StringEncoder(event.SessionID)
	case PartitionByAgentID:
		return sarama.StringEncoder(event.AgentID)
	default:
		return nil
	}
}

// drainErrors reads from the producer Errors channel and logs failures.
func (kp *KafkaPublisher) drainErrors() {
	defer kp.wg.Done()

	for prodErr := range kp.producer.Errors() {
		kp.logger.Error("kafka publish failed",
			"topic", prodErr.Msg.Topic,
			"error", prodErr.Err.Error(),
		)
	}
}

// buildSaramaConfig translates KafkaConfig into a sarama.Config.
func buildSaramaConfig(cfg *KafkaConfig) (*sarama.Config, error) {
	sc := sarama.NewConfig()
	sc.Producer.Return.Errors = true
	sc.Producer.Partitioner = newSessionEventPartitioner(cfg.PartitionStrategy)

	if err := configureAcks(sc, cfg.Acks); err != nil {
		return nil, err
	}

	configureCompression(sc, cfg.Compression)
	configureProducerTuning(sc, cfg)
	configureSASL(sc, cfg.SASL)
	configureTLS(sc, cfg.TLS)

	return sc, nil
}

func configureAcks(sc *sarama.Config, acks string) error {
	switch acks {
	case "0":
		sc.Producer.RequiredAcks = sarama.NoResponse
	case "1":
		sc.Producer.RequiredAcks = sarama.WaitForLocal
	case "all", "":
		sc.Producer.RequiredAcks = sarama.WaitForAll
	default:
		return fmt.Errorf("unsupported acks value: %s", acks)
	}
	return nil
}

func configureCompression(sc *sarama.Config, compression string) {
	switch compression {
	case "gzip":
		sc.Producer.Compression = sarama.CompressionGZIP
	case "snappy":
		sc.Producer.Compression = sarama.CompressionSnappy
	case "lz4":
		sc.Producer.Compression = sarama.CompressionLZ4
	default:
		sc.Producer.Compression = sarama.CompressionNone
	}
}

func configureProducerTuning(sc *sarama.Config, cfg *KafkaConfig) {
	if cfg.Retries > 0 {
		sc.Producer.Retry.Max = cfg.Retries
	}
	if cfg.BatchSize > 0 {
		sc.Producer.Flush.Bytes = cfg.BatchSize
	}
	if cfg.LingerMs > 0 {
		sc.Producer.Flush.Frequency = durationFromMs(cfg.LingerMs)
	}
}

func configureSASL(sc *sarama.Config, sasl *SASLConfig) {
	if sasl == nil {
		return
	}
	sc.Net.SASL.Enable = true
	sc.Net.SASL.User = sasl.Username
	sc.Net.SASL.Password = sasl.Password
	sc.Net.SASL.Mechanism = sarama.SASLMechanism(sasl.Mechanism)
}

func configureTLS(sc *sarama.Config, tlsCfg *TLSConfig) {
	if tlsCfg == nil || !tlsCfg.Enable {
		return
	}
	sc.Net.TLS.Enable = true
	if tlsCfg.Config != nil {
		sc.Net.TLS.Config = tlsCfg.Config
	} else {
		sc.Net.TLS.Config = &tls.Config{MinVersion: tls.VersionTLS12} //nolint:gosec // default TLS config
	}
}
