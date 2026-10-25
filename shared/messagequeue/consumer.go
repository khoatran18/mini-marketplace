package messagequeue

import (
	"context"

	"mini-marketplace/shared/messagequeue/kafkaimpl"
)

type Consumer interface {
	Consume(ctx context.Context, topic, groupID string, handler kafkaimpl.MessageHandler) error
}
