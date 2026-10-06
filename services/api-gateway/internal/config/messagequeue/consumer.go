package messagequeue

import (
	"api-gateway/internal/config/messagequeue/kafkaimpl"
	"context"
)

type Consumer interface {
	Consume(ctx context.Context, topic, groupID string, handler kafkaimpl.MessageHandler) error
}
