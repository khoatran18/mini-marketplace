package kafkaimpl

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/segmentio/kafka-go"
)

type KafkaClient struct {
	Client *kafka.Client
	mu     sync.Mutex
	// replicationFactor for topics created by EnsureTopicExist: min(3, brokers).
	replicationFactor int
}

// maxReplicationFactor is the replication factor used when at least that many brokers are configured.
const maxReplicationFactor = 3

// replicationFactorFor returns min(3, number of brokers); entries may themselves be comma lists. Never below 1.
func replicationFactorFor(brokers []string) int {
	n := 0
	for _, b := range brokers {
		for _, a := range strings.Split(b, ",") {
			if strings.TrimSpace(a) != "" {
				n++
			}
		}
	}
	if n > maxReplicationFactor {
		n = maxReplicationFactor
	}
	if n < 1 {
		n = 1
	}
	return n
}

func NewKafkaClient(brokersList []string) *KafkaClient {
	return &KafkaClient{
		Client: &kafka.Client{
			Addr: kafka.TCP(brokersList...),
		},
		replicationFactor: replicationFactorFor(brokersList),
	}
}

func (c *KafkaClient) createTopic(ctx context.Context, topic string) error {
	createTopicRequest := &kafka.CreateTopicsRequest{
		Addr: c.Client.Addr,
		Topics: []kafka.TopicConfig{
			kafka.TopicConfig{
				Topic:             topic,
				NumPartitions:     3,
				ReplicationFactor: c.replicationFactor,
			},
		},
	}

	createTopicResponse, err := c.Client.CreateTopics(ctx, createTopicRequest)
	log.Printf("CreateTopicResponse: %+v\n", createTopicResponse)
	if err != nil {
		return err
	}

	if e, ok := createTopicResponse.Errors[topic]; ok && e != nil {
		if errors.Is(e, kafka.TopicAlreadyExists) {
			log.Printf("Topic %s already exists, err: %v", topic, e)
			log.Printf("Kafka topic %v already exists\n", topic)
			return nil
		}
		return e
	}
	return nil
}

func (c *KafkaClient) checkTopicExist(ctx context.Context, topic string) (bool, error) {
	metadataRequest := &kafka.MetadataRequest{
		Addr:   c.Client.Addr,
		Topics: []string{topic},
	}
	metadataResponse, err := c.Client.Metadata(ctx, metadataRequest)
	if err != nil {
		log.Printf("Kafka topic %v does not exist\n", topic)
		return false, err
	}
	log.Printf("MetadataResponse: %+v\n", metadataResponse)
	for _, top := range metadataResponse.Topics {
		if top.Name == topic {
			return topicReady(top), nil
		}
	}
	log.Printf("Can not find Kafka topic %v \n", topic)
	return false, nil
}

// topicReady: topic có đủ partition, mỗi partition đã có leader và không lỗi. Metadata của topic vừa bị auto-create (hoặc
// chưa được tạo) vẫn trả về entry mang tên topic kèm lỗi "Unknown Topic Or Partition" và 0 partition: không được coi là đã tồn tại,
// nếu không consumer join group lúc đó sẽ được chia 0 partition và đứng yên mãi.
func topicReady(t kafka.Topic) bool {
	if t.Error != nil || len(t.Partitions) == 0 {
		return false
	}
	for _, p := range t.Partitions {
		if p.Error != nil || p.Leader.Host == "" {
			return false
		}
	}
	return true
}

// waitTopicReady chờ (tối đa timeout) tới khi topic sẵn sàng cho consumer / producer.
func (c *KafkaClient) waitTopicReady(ctx context.Context, topic string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	for {
		if ok, _ := c.checkTopicExist(ctx, topic); ok {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("kafka topic %s chưa sẵn sàng sau %s: %w", topic, timeout, ctx.Err())
		case <-ticker.C:
		}
	}
}

func (c *KafkaClient) EnsureTopicExist(ctx context.Context, topic string) error {
	exist, _ := c.checkTopicExist(ctx, topic)
	if exist == true {
		log.Printf("Kafka topic %v already exists\n", topic)
		return nil
	}
	if err := c.createTopic(ctx, topic); err != nil {
		log.Printf("Kafka topic %v not exists, created failed\n", topic)
		return err
	}
	if err := c.waitTopicReady(ctx, topic, 30*time.Second); err != nil {
		return err
	}
	log.Printf("Kafka topic %v not exists, created successfully\n", topic)
	return nil
}
