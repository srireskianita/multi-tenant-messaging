package tenant

import (
	"context"
	"log"
	"sync"
	"sync/atomic"

	amqp "github.com/rabbitmq/amqp091-go"
)

type WorkerPool struct {
	numWorkers int32
	jobChan    chan amqp.Delivery
	wg         sync.WaitGroup
	ctx        context.Context
	cancel     context.CancelFunc
}

func NewWorkerPool(ctx context.Context, workers int) *WorkerPool {
	ctx, cancel := context.WithCancel(ctx)

	return &WorkerPool{
		numWorkers: int32(workers),
		jobChan:    make(chan amqp.Delivery, 100),
		wg:         sync.WaitGroup{},
		ctx:        ctx,
		cancel:     cancel,
	}
}

func (wp *WorkerPool) Start(tc *TenantConsumer) {
	num := atomic.LoadInt32(&wp.numWorkers)
	log.Printf("Starting %d workers for tenant %s", num, tc.tenantID)

	for i := 0; i < int(num); i++ {
		wp.wg.Add(1)
		go func(workerID int) {
			defer wp.wg.Done()
			for {
				select {
				case msg := <-wp.jobChan:
					err := tc.saveMessage(msg.Body)
					if err != nil {
						log.Printf("Worker %d: failed to save message for tenant %s: %v", workerID, tc.tenantID, err)
						msg.Nack(false, true)
					} else {
						log.Printf("Worker %d: processed message for tenant %s: %s", workerID, tc.tenantID, string(msg.Body))
						msg.Ack(false)
					}
				case <-wp.ctx.Done():
					log.Printf("Worker %d stopping for tenant %s", workerID, tc.tenantID)
					return
				}
			}
		}(i)
	}
}
