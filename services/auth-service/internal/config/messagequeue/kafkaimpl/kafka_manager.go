package kafkaimpl

import (
	"sync"
	"time"

	"github.com/segmentio/kafka-go"
)

type KafkaManager struct {
	brokers []string
	mu      sync.RWMutex
	writers map[string]*kafka.Writer
	readers map[string]*kafka.Reader
}

func NewKafkaManager(brokers []string) *KafkaManager {
	return &KafkaManager{
		brokers: brokers,
		writers: make(map[string]*kafka.Writer),
		readers: make(map[string]*kafka.Reader),
	}
}

////////////////////////////////// For Producer ////////////////////////////////////////////////////////////

func (m *KafkaManager) newWriterForTopic(topic string, balancer kafka.Balancer) *kafka.Writer {
	return &kafka.Writer{
		Addr:         kafka.TCP(m.brokers...),
		Topic:        topic,
		Balancer:     balancer,
		BatchBytes:   1e6,
		BatchTimeout: 500 * time.Millisecond,
	}
}

func (m *KafkaManager) NewWriter(topic string, balancer kafka.Balancer) *kafka.Writer {
	m.mu.Lock()
	defer m.mu.Unlock()

	if writer, ok := m.writers[topic]; ok {
		return writer
	}

	writer := m.newWriterForTopic(topic, balancer)
	m.writers[topic] = writer
	return writer
}

func (m *KafkaManager) CloseWriterAll() error {
	var firstErr error // Close all writer even when have an error with a topic
	for key, writer := range m.writers {
		if err := writer.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		delete(m.writers, key)
	}
	return firstErr
}

////////////////////////////////// For Consumer ////////////////////////////////////////////////////////////

func (m *KafkaManager) newReaderForTopic(topic string, groupID string) *kafka.Reader {
	return kafka.NewReader(kafka.ReaderConfig{
		Brokers:  m.brokers,
		Topic:    topic,
		GroupID:  groupID,
		MinBytes: 10e3,
		MaxBytes: 10e6,
		MaxWait:  2 * time.Second,
	})
}

func (m *KafkaManager) NewReader(topic string, groupID string) *kafka.Reader {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := topic + ":" + groupID
	if reader, ok := m.readers[key]; ok {
		return reader
	}

	reader := m.newReaderForTopic(topic, groupID)
	m.readers[key] = reader
	return reader
}

func (m *KafkaManager) CloseReaderAll() error {
	var firstErr error
	for key, reader := range m.readers {
		if err := reader.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		delete(m.readers, key)
	}
	return firstErr
}
