package tenant

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"
)

type TenantManager struct {
	conn           *amqp.Connection
	consumers      map[uuid.UUID]*TenantConsumer
	channels       map[uuid.UUID]*amqp.Channel
	db             *pgxpool.Pool
	mu             sync.Mutex
	defaultWorkers int
}

func NewTenantManager(conn *amqp.Connection, db *pgxpool.Pool, defaultWorkers int) *TenantManager {
	return &TenantManager{
		conn:           conn,
		consumers:      make(map[uuid.UUID]*TenantConsumer),
		channels:       make(map[uuid.UUID]*amqp.Channel),
		db:             db,
		defaultWorkers: defaultWorkers,
	}
}

func (tm *TenantManager) CreateTenant(ctx context.Context, tenantID uuid.UUID) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if _, exists := tm.consumers[tenantID]; exists {
		return fmt.Errorf("tenant %s already exists", tenantID)
	}

	ch, err := tm.conn.Channel()
	if err != nil {
		return fmt.Errorf("failed to open channel: %w", err)
	}

	queueName := fmt.Sprintf("tenant_%s_queue", tenantID)

	_, err = ch.QueueDeclare(
		queueName,
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		ch.Close()
		return fmt.Errorf("failed to declare queue: %w", err)
	}

	deliveries, err := ch.Consume(
		queueName,
		fmt.Sprintf("tenant_consumer_%s", tenantID),
		false,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		ch.Close()
		return fmt.Errorf("failed to consume: %w", err)
	}

	consumer := NewTenantConsumer(tenantID, queueName, ch, deliveries, tm.defaultWorkers, tm.db)
	tm.consumers[tenantID] = consumer
	tm.channels[tenantID] = ch

	log.Printf("✅ Tenant %s created and consumer started", tenantID)
	return nil
}

func (tm *TenantManager) DeleteTenant(ctx context.Context, tenantID uuid.UUID) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	consumer, ok := tm.consumers[tenantID]
	if !ok {
		return fmt.Errorf("tenant %s not found", tenantID)
	}

	ch, ok := tm.channels[tenantID]
	if !ok {
		return fmt.Errorf("channel for tenant %s not found", tenantID)
	}

	if err := consumer.Stop(); err != nil {
		log.Printf("Error stopping consumer for tenant %s: %v", tenantID, err)
	}

	queueName := fmt.Sprintf("tenant_%s_queue", tenantID)

	_, err := ch.QueueDelete(
		queueName,
		false,
		false,
		false,
	)
	if err != nil {
		log.Printf("Failed to delete queue %s: %v", queueName, err)
	}

	ch.Close()

	delete(tm.consumers, tenantID)
	delete(tm.channels, tenantID)

	log.Printf("❌ Tenant %s deleted and consumer stopped", tenantID)
	return nil
}

// UpdateConcurrency updates the worker count dynamically for a tenant consumer
func (tm *TenantManager) UpdateConcurrency(tenantID uuid.UUID, concurrency int) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	consumer, ok := tm.consumers[tenantID]
	if !ok {
		return fmt.Errorf("tenant %s not found", tenantID)
	}

	consumer.UpdateWorkerCount(concurrency)
	log.Printf("🔄 Updated concurrency for tenant %s to %d", tenantID, concurrency)
	return nil
}


func (tm *TenantManager) GetMessagesPaginated(ctx context.Context, tenantID string, cursor string, limit int) ([]Message, string, error) {
    if limit <= 0 || limit > 100 {
        limit = 10
    }

    var messages []Message

    query := `
        SELECT id, tenant_id, payload, created_at
        FROM messages
        WHERE tenant_id = $1
    `
    args := []interface{}{tenantID}
    paramIndex := 2

    if cursor != "" {
        cursorUUID, err := uuid.Parse(cursor)
        if err != nil {
            return nil, "", fmt.Errorf("invalid cursor UUID format: %w", err)
        }
        query += fmt.Sprintf(" AND id > $%d", paramIndex)
        args = append(args, cursorUUID)
        paramIndex++
    }

    query += fmt.Sprintf(" ORDER BY id ASC LIMIT $%d", paramIndex)
    args = append(args, limit+1)

    rows, err := tm.db.Query(ctx, query, args...)
    if err != nil {
        return nil, "", err
    }
    defer rows.Close()

    for rows.Next() {
        var msg Message
        if err := rows.Scan(&msg.ID, &msg.TenantID, &msg.Payload, &msg.CreatedAt); err != nil {
            return nil, "", err
        }
        messages = append(messages, msg)
    }

    var nextCursor string
    if len(messages) > limit {
        nextCursor = messages[limit].ID.String()
        messages = messages[:limit]
    }

    return messages, nextCursor, nil
}


func (tm *TenantManager) PublishMessage(ctx context.Context, tenantID uuid.UUID, payload string) error {
    tm.mu.Lock()
    ch, ok := tm.channels[tenantID]
    tm.mu.Unlock()
    if !ok {
        return fmt.Errorf("channel for tenant %s not found", tenantID)
    }

    queueName := fmt.Sprintf("tenant_%s_queue", tenantID)

    err := ch.PublishWithContext(ctx,
        "",        // exchange
        queueName, // routing key (queue name)
        false,     // mandatory
        false,     // immediate
        amqp.Publishing{
            ContentType: "application/json",
            Body:        []byte(payload),
        },
    )
    if err != nil {
        return fmt.Errorf("failed to publish message: %w", err)
    }

    return nil
}
