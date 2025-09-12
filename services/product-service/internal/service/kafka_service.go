package service

import (
	"encoding/json"
	"product-service/pkg/dto"

	"github.com/segmentio/kafka-go"
)

func (s *ProductService) ValidateProductInventory(ctx, msg *kafka.Message) error {
	var eventDTO dto.ItemEvent
	if err := json.Unmarshal(msg.Value, &eventDTO); err != nil {
		return err
	}

}
