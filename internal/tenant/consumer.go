package tenant

import (
	"context"
	"log"
	"sync/atomic"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"
)

// TenantConsumer handles consuming messages from RabbitMQ and storing them in PostgreSQL.
type TenantConsumer struct {
	tenantID   uuid.UUID
	queueName  string
	channel    *amqp.Channel
	deliveries <-chan amqp.Delivery
	workerPool *WorkerPool
	cancelFunc context.CancelFunc
	dbPool     *pgxpool.Pool
}

func NewTenantConsumer(tenantID uuid.UUID, queueName string, ch *amqp.Channel, deliveries <-chan amqp.Delivery, workers int, dbPool *pgxpool.Pool) *TenantConsumer {
	ctx, cancel := context.WithCancel(context.Background())

	wp := NewWorkerPool(ctx, workers)

	tc := &TenantConsumer{
		tenantID:   tenantID,
		queueName:  queueName,
		channel:    ch,
		deliveries: deliveries,
		workerPool: wp,
		cancelFunc: cancel,
		dbPool:     dbPool,
	}

	go tc.routeMessages()
	wp.Start(tc)

	return tc
}

func (tc *TenantConsumer) routeMessages() {
	for {
		select {
		case msg, ok := <-tc.deliveries:
			if !ok {
				log.Printf("Deliveries channel closed for tenant %s", tc.tenantID)
				return
			}
			select {
			case tc.workerPool.jobChan <- msg:
			case <-tc.workerPool.ctx.Done():
				return
			}
		case <-tc.workerPool.ctx.Done():
			return
		}
	}
}

func (tc *TenantConsumer) saveMessage(payload []byte) error {
	query := `
		INSERT INTO messages (id, tenant_id, payload)
		VALUES ($1, $2, $3)
	`

	_, err := tc.dbPool.Exec(context.Background(), query, uuid.New(), tc.tenantID, payload)
	return err
}

func (tc *TenantConsumer) Stop() error {
	log.Printf("Stopping consumer for tenant %s", tc.tenantID)
	tc.cancelFunc()
	tc.workerPool.cancel()
	tc.workerPool.wg.Wait()
	if err := tc.channel.Cancel("", false); err != nil {
		log.Printf("Failed to cancel consumer for tenant %s: %v", tc.tenantID, err)
	}
	return tc.channel.Close()
}

func (tc *TenantConsumer) UpdateWorkerCount(workers int) {
	log.Printf("Updating worker count for tenant %s to %d", tc.tenantID, workers)
	tc.workerPool.cancel()
	tc.workerPool.wg.Wait()

	ctx, cancel := context.WithCancel(context.Background())
	tc.workerPool.ctx = ctx
	tc.workerPool.cancel = cancel
	atomic.StoreInt32(&tc.workerPool.numWorkers, int32(workers))
	tc.workerPool.Start(tc)
}
