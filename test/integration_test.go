package test

import (
	"context"
	"log"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/srireskianita/multi-tenant-messaging/internal/tenant"
)

var (
	dbPool  *pgxpool.Pool
	rmqConn *amqp.Connection
	tm      *tenant.TenantManager
)

func TestMain(m *testing.M) {
	// Setup docker pool
	pool, err := dockertest.NewPool("")
	if err != nil {
		log.Fatalf("Could not connect to docker: %s", err)
	}

	// Run Postgres container
	pgResource, err := pool.RunWithOptions(&dockertest.RunOptions{
		Repository: "postgres",
		Tag:        "13",
		Env: []string{
			"POSTGRES_USER=postgres",
			"POSTGRES_PASSWORD=secret",
			"POSTGRES_DB=testdb",
		},
	}, func(config *docker.HostConfig) {
		config.AutoRemove = true
		config.RestartPolicy = docker.RestartPolicy{Name: "no"}
	})
	if err != nil {
		log.Fatalf("Could not start postgres container: %s", err)
	}

	pgPort := pgResource.GetPort("5432/tcp")
	dbURL := "postgres://postgres:secret@localhost:" + pgPort + "/testdb?sslmode=disable"

	// Wait for Postgres to be ready
	if err = pool.Retry(func() error {
		var err error
		dbPool, err = pgxpool.New(context.Background(), dbURL)
		if err != nil {
			return err
		}
		return dbPool.Ping(context.Background())
	}); err != nil {
		log.Fatalf("Could not connect to postgres: %s", err)
	}

	// Run RabbitMQ container
	rmqResource, err := pool.RunWithOptions(&dockertest.RunOptions{
		Repository: "rabbitmq",
		Tag:        "3.9-management",
		Env: []string{
			"RABBITMQ_DEFAULT_USER=guest",
			"RABBITMQ_DEFAULT_PASS=guest",
		},
	}, func(config *docker.HostConfig) {
		config.AutoRemove = true
		config.RestartPolicy = docker.RestartPolicy{Name: "no"}
	})
	if err != nil {
		log.Fatalf("Could not start rabbitmq container: %s", err)
	}

	rmqPort := rmqResource.GetPort("5672/tcp")
	rmqURL := "amqp://guest:guest@localhost:" + rmqPort + "/"

	// Wait for RabbitMQ to be ready
	if err = pool.Retry(func() error {
		rmqConn, err = amqp.Dial(rmqURL)
		return err
	}); err != nil {
		log.Fatalf("Could not connect to rabbitmq: %s", err)
	}

	// Initialize TenantManager with concurrency 5 (example)
	tm = tenant.NewTenantManager(rmqConn, dbPool, 5)

	code := m.Run()

	// Cleanup
	if err := rmqConn.Close(); err != nil {
		log.Printf("Error closing RabbitMQ connection: %v", err)
	}
	dbPool.Close()

	if err := pool.Purge(pgResource); err != nil {
		log.Printf("Could not purge postgres container: %v", err)
	}
	if err := pool.Purge(rmqResource); err != nil {
		log.Printf("Could not purge rabbitmq container: %v", err)
	}

	os.Exit(code)
}

func TestTenantLifecycle(t *testing.T) {
	ctx := context.Background()
	tenantID := uuid.New()

	// Create tenant (declare queue and start consumer)
	if err := tm.CreateTenant(ctx, tenantID); err != nil {
		t.Fatalf("Failed to create tenant: %v", err)
	}

	// Update concurrency config
	newConcurrency := 3
	if err := tm.UpdateConcurrency(tenantID, newConcurrency); err != nil {
		t.Fatalf("Failed to update concurrency: %v", err)
	}

	// Publish message to tenant queue
	ch, err := rmqConn.Channel()
	if err != nil {
		t.Fatalf("Failed to open RabbitMQ channel: %v", err)
	}
	defer ch.Close()

	queueName := "tenant_" + tenantID.String() + "_queue"
	payload := []byte(`{"test":"integration"}`)

	err = ch.Publish(
		"",
		queueName,
		false,
		false,
		amqp.Publishing{
			ContentType: "application/json",
			Body:        payload,
		},
	)
	if err != nil {
		t.Fatalf("Failed to publish message: %v", err)
	}

	// Wait for consumer to process message and save to DB
	time.Sleep(2 * time.Second)

	// Verify message stored in DB
	rows, err := dbPool.Query(ctx, "SELECT payload FROM messages WHERE tenant_id=$1", tenantID)
	if err != nil {
		t.Fatalf("Failed to query messages: %v", err)
	}
	defer rows.Close()

	found := false
	for rows.Next() {
		var dbPayload []byte
		if err := rows.Scan(&dbPayload); err != nil {
			t.Fatalf("Failed to scan row: %v", err)
		}
		if string(dbPayload) == string(payload) {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("Published message not found in DB")
	}

	// Delete tenant (stop consumer and delete queue)
	if err := tm.DeleteTenant(ctx, tenantID); err != nil {
		t.Fatalf("Failed to delete tenant: %v", err)
	}
}
