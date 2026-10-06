package service

import (
	"api-gateway/pkg/dto"
	"context"
	"encoding/json"
	"fmt"

	"github.com/segmentio/kafka-go"
)

func (s *APIGatewayService) AddChaPwdVerToRedis(ctx context.Context, msg *kafka.Message) error {

	var eventDTO dto.ChangePwdKafkaEvent
	if err := json.Unmarshal(msg.Value, &eventDTO); err != nil {
		return err
	}
	period := s.PwdVersionTTL

	key := fmt.Sprintf("%d:pwd_version", eventDTO.UserID)
	if err := s.RedisClient.Set(ctx, key, eventDTO.PwdVersion, period).Err(); err != nil {
		return err
	}
	return nil
}
