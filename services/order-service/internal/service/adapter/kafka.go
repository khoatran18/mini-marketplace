package adapter

import (
	"encoding/json"
	"order-service/pkg/outbox"
)

func CreOrdEvesModelToKafkaEvent(orderModel *outbox.CreateOrderEvent) (*outbox.CreateOrderKafkaEvent, error) {
	itemsModel := orderModel.Items
	var itemsKafkaEvent []*outbox.ItemEvent
	if err := json.Unmarshal(itemsModel, &itemsKafkaEvent); err != nil {
		return nil, err
	}
	//log.Printf("ItemsModel before convert: %+v\n", itemsModel)
	//log.Printf("ItemsModel after convert: %+v\n", itemsKafkaEvent)
	//for _, item := range itemsKafkaEvent {
	//	log.Printf("Item: %+v\n", item)
	//}
	return &outbox.CreateOrderKafkaEvent{
		OrderID: orderModel.OrderID,
		Items:   itemsKafkaEvent,
	}, nil
}
