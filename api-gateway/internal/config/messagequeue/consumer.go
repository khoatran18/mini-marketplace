package messagequeue

import (
	"auth-service/internal/config/messagequeue/kafkaimpl"
	"context"
)

type Consumer interface {
	Consume(ctx context.Context, topic, groupID string, handler kafkaimpl.MessageHandler) error
}
