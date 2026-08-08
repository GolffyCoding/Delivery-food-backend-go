package outbox

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/opendelivery/opendelivery/pkg/eventbus"
	"github.com/uptrace/bun"
	"go.uber.org/zap"
)

type Message struct {
	bun.BaseModel `bun:"table:outbox_messages"`

	ID          uuid.UUID       `bun:",pk,type:uuid,default:gen_random_uuid()"`
	Aggregate   string          `bun:",notnull"`
	AggregateID uuid.UUID       `bun:",type:uuid,notnull"`
	EventType   string          `bun:",notnull"`
	Payload     json.RawMessage `bun:",notnull"`
	Status      string          `bun:",default:'pending'"`
	CreatedAt   time.Time       `bun:",nullzero,default:now()"`
	PublishedAt *time.Time
}

func NewMessage(aggregate string, aggregateID uuid.UUID, eventType string, payload interface{}) (*Message, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal outbox payload: %w", err)
	}
	return &Message{
		ID:          uuid.New(),
		Aggregate:   aggregate,
		AggregateID: aggregateID,
		EventType:   eventType,
		Payload:     data,
	}, nil
}

type Repository interface {
	InsertInTx(ctx context.Context, tx bun.Tx, messages ...*Message) error
	AppendInTx(ctx context.Context, tx bun.Tx, aggregate string, aggregateID uuid.UUID, eventType string, payload interface{}) error
	GetUnpublished(ctx context.Context, limit int) ([]Message, error)
	MarkAsPublished(ctx context.Context, ids []uuid.UUID) error
	MarkAsFailed(ctx context.Context, id uuid.UUID) error
}

type postgresRepository struct {
	db *bun.DB
}

func NewPostgresRepository(db *bun.DB) Repository {
	return &postgresRepository{db: db}
}

func (r *postgresRepository) InsertInTx(ctx context.Context, tx bun.Tx, messages ...*Message) error {
	if len(messages) == 0 {
		return nil
	}
	_, err := tx.NewInsert().Model(&messages).Exec(ctx)
	return err
}

// AppendInTx builds an outbox message and inserts it using the caller's transaction,
// satisfying order.OutboxAppender (and any other aggregate's equivalent interface).
func (r *postgresRepository) AppendInTx(ctx context.Context, tx bun.Tx, aggregate string, aggregateID uuid.UUID, eventType string, payload interface{}) error {
	msg, err := NewMessage(aggregate, aggregateID, eventType, payload)
	if err != nil {
		return err
	}
	return r.InsertInTx(ctx, tx, msg)
}

func (r *postgresRepository) GetUnpublished(ctx context.Context, limit int) ([]Message, error) {
	var messages []Message
	err := r.db.NewSelect().
		Model(&messages).
		Where("status = 'pending'").
		Order("created_at ASC").
		Limit(limit).
		Scan(ctx)
	return messages, err
}

func (r *postgresRepository) MarkAsPublished(ctx context.Context, ids []uuid.UUID) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := r.db.NewUpdate().
		Model((*Message)(nil)).
		Set("status = 'published'").
		Set("published_at = NOW()").
		Where("id IN (?)", bun.In(ids)).
		Exec(ctx)
	return err
}

func (r *postgresRepository) MarkAsFailed(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.NewUpdate().
		Model((*Message)(nil)).
		Set("status = 'failed'").
		Where("id = ?", id).
		Exec(ctx)
	return err
}

// PublisherWorker polls unpublished outbox rows and relays them to the event
// bus, decoupling "commit the DB write" from "notify the world" so a NATS
// outage can never silently drop an order-created event.
type PublisherWorker struct {
	repo      Repository
	eventBus  *eventbus.EventBus
	logger    *zap.Logger
	batchSize int
}

func NewPublisherWorker(repo Repository, eventBus *eventbus.EventBus, logger *zap.Logger) *PublisherWorker {
	return &PublisherWorker{repo: repo, eventBus: eventBus, logger: logger, batchSize: 100}
}

func (w *PublisherWorker) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.processBatch(ctx)
		}
	}
}

func (w *PublisherWorker) processBatch(ctx context.Context) {
	messages, err := w.repo.GetUnpublished(ctx, w.batchSize)
	if err != nil {
		w.logger.Error("failed to fetch outbox messages", zap.Error(err))
		return
	}
	if len(messages) == 0 {
		return
	}

	var publishedIDs []uuid.UUID
	for _, msg := range messages {
		subject := fmt.Sprintf("%s.%s", msg.Aggregate, msg.EventType)
		if err := w.eventBus.Publish(subject, msg.Payload, msg.ID.String()); err != nil {
			w.logger.Error("failed to publish outbox message", zap.String("id", msg.ID.String()), zap.Error(err))
			_ = w.repo.MarkAsFailed(ctx, msg.ID)
			continue
		}
		publishedIDs = append(publishedIDs, msg.ID)
	}

	if len(publishedIDs) > 0 {
		if err := w.repo.MarkAsPublished(ctx, publishedIDs); err != nil {
			w.logger.Error("failed to mark outbox as published", zap.Error(err))
		}
	}
}
