package kafkaimpl

import (
	"context"
	"errors"
	"log"
	"sync"

	"github.com/segmentio/kafka-go"
)

type KafkaClient struct {
	Client *kafka.Client
	mu     sync.Mutex
}

func NewKafkaClient(brokersList []string) *KafkaClient {
	return &KafkaClient{
		Client: &kafka.Client{
			Addr: kafka.TCP(brokersList...),
		},
	}
}

func (c *KafkaClient) createTopic(ctx context.Context, topic string) error {
	createTopicRequest := &kafka.CreateTopicsRequest{
		Addr: c.Client.Addr,
		Topics: []kafka.TopicConfig{
			kafka.TopicConfig{
				Topic:             topic,
				NumPartitions:     -1,
				ReplicationFactor: -1,
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
		return err
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
			return true, nil
		}
	}
	log.Printf("Can not find Kafka topic %v \n", topic)
	return false, nil
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
	log.Printf("Kafka topic %v not exists, created successfully\n", topic)
	return nil
}

func (c *KafkaClient) CreateTopicByLeader(ctx context.Context, topic string) error {
	// Test
	conn, err := kafka.DialLeader(context.Background(), "tcp", "localhost:9092", topic, 0)
	defer conn.Close()
	if err != nil {
		return err
	}
	return nil
}
