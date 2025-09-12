package messagequeue

import (
	"context"
	"product-service/internal/config/messagequeue/kafkaimpl"
)

type Consumer interface {
	Consume(ctx context.Context, topic, groupID string, handler kafkaimpl.MessageHandler) error
}
