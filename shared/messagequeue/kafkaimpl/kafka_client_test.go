package kafkaimpl

import (
	"testing"

	"github.com/segmentio/kafka-go"
)

func TestTopicReady(t *testing.T) {
	leader := kafka.Broker{Host: "broker1", Port: 9092, ID: 1}
	ok := kafka.Partition{ID: 0, Leader: leader}
	cases := []struct {
		name  string
		topic kafka.Topic
		want  bool
	}{
		{"đủ partition và leader", kafka.Topic{Name: "t", Partitions: []kafka.Partition{ok, ok}}, true},
		{"topic vừa auto-create: Unknown Topic Or Partition, 0 partition", kafka.Topic{Name: "t", Error: kafka.UnknownTopicOrPartition}, false},
		{"không có partition", kafka.Topic{Name: "t"}, false},
		{"partition chưa có leader", kafka.Topic{Name: "t", Partitions: []kafka.Partition{ok, {ID: 1}}}, false},
		{"partition lỗi", kafka.Topic{Name: "t", Partitions: []kafka.Partition{{ID: 0, Leader: leader, Error: kafka.LeaderNotAvailable}}}, false},
	}
	for _, c := range cases {
		if got := topicReady(c.topic); got != c.want {
			t.Errorf("%s: topicReady = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestReplicationFactorFor(t *testing.T) {
	cases := []struct {
		brokers []string
		want    int
	}{
		{nil, 1},
		{[]string{"k1:9092"}, 1},
		{[]string{"k1:9092", "k2:9092"}, 2},
		{[]string{"k1:9092", "k2:9092", "k3:9092"}, 3},
		{[]string{"k1:9092", "k2:9092", "k3:9092", "k4:9092", "k5:9092"}, 3},
		{[]string{"k1:9092,k2:9092,k3:9092"}, 3},
		{[]string{" k1:9092 ", ""}, 1},
	}
	for _, c := range cases {
		if got := replicationFactorFor(c.brokers); got != c.want {
			t.Errorf("replicationFactorFor(%v) = %d, want %d", c.brokers, got, c.want)
		}
	}
}
