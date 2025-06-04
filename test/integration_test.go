package integration_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	amqp091 "github.com/rabbitmq/amqp091-go"

	"github.com/srireskianita/multi-tenant-messaging/internal/tenant"
)

var (
	sqlDB      *sql.DB
	pgPool     *pgxpool.Pool
	rabbitConn *amqp091.Connection
)

func TestMain(m *testing.M) {
	pool, err := dockertest.NewPool("")
	if err != nil {
		log.Fatalf("Could not connect to Docker: %s", err)
	}

	resource, err := pool.RunWithOptions(&dockertest.RunOptions{
		Repository: "postgres",
		Tag:        "13",
		Env: []string{
			"POSTGRES_USER=test",
			"POSTGRES_PASSWORD=secret",
			"POSTGRES_DB=testdb",
		},
	}, func(config *docker.HostConfig) {
		config.AutoRemove = true
		config.RestartPolicy = docker.RestartPolicy{Name: "no"}
	})
	if err != nil {
		log.Fatalf("Could not start resource: %s", err)
	}
	resource.Expire(120)

	dsn := fmt.Sprintf("postgres://test:secret@localhost:%s/testdb?sslmode=disable", resource.GetPort("5432/tcp"))

	err = pool.Retry(func() error {
		sqlDB, err = sql.Open("postgres", dsn)
		if err != nil {
			return err
		}
		return sqlDB.Ping()
	})
	if err != nil {
		log.Fatalf("Could not connect to database: %s", err)
	}

	if err := runMigrations(sqlDB); err != nil {
		log.Fatalf("Could not run migrations: %s", err)
	}

	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		log.Fatalf("Failed to parse pgxpool config: %v", err)
	}
	pgPool, err = pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		log.Fatalf("Failed to connect pgxpool: %v", err)
	}

	rabbitConn, err = amqp091.Dial("amqp://guest:guest@localhost:5672/")
	if err != nil {
		log.Fatalf("Failed to connect RabbitMQ: %v", err)
	}

	code := m.Run()

	rabbitConn.Close()
	pgPool.Close()
	_ = pool.Purge(resource)
	os.Exit(code)
}

func runMigrations(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS messages (
		id UUID,
		tenant_id UUID NOT NULL,
		payload JSONB,
		created_at TIMESTAMPTZ DEFAULT NOW(),
		PRIMARY KEY (tenant_id, id)
	) PARTITION BY LIST (tenant_id);
	`
	_, err := db.Exec(schema)
	return err
}

func TestTenantLifecycle(t *testing.T) {
	ctx := context.Background()
	tm := tenant.NewTenantManager(rabbitConn, pgPool, 5)
	tenantID := uuid.New()

	// Create tenant
	err := tm.CreateTenant(ctx, tenantID)
	if err != nil {
		t.Fatalf("failed to create tenant: %v", err)
	}

	// Publish a message to verify the tenant exists
	payload := `{"msg":"lifecycle test"}`
	err = tm.PublishMessage(ctx, tenantID, payload)
	if err != nil {
		t.Fatalf("failed to publish message after tenant creation: %v", err)
	}

	// Delete tenant
	err = tm.DeleteTenant(ctx, tenantID)
	if err != nil {
		t.Fatalf("failed to delete tenant: %v", err)
	}

	// Try to publish again after deletion — should fail
	err = tm.PublishMessage(ctx, tenantID, payload)
	if err == nil {
		t.Fatal("expected error when publishing to deleted tenant, but got none")
	}
}

func TestMessagePublishConsume(t *testing.T) {
	ctx := context.Background()
	tm := tenant.NewTenantManager(rabbitConn, pgPool, 5)
	tenantID := uuid.New()

	err := tm.CreateTenant(ctx, tenantID)
	if err != nil {
		t.Fatalf("failed to create tenant: %v", err)
	}

	payload := `{"msg":"Hello from test"}`
	err = tm.PublishMessage(ctx, tenantID, payload)
	if err != nil {
		t.Fatalf("failed to publish message: %v", err)
	}

	time.Sleep(2 * time.Second) // wait for message to be processed

	msgs, _, err := tm.GetMessagesPaginated(ctx, tenantID.String(), "", 10)
	if err != nil {
		t.Fatalf("failed to get messages: %v", err)
	}
	if len(msgs) == 0 {
		t.Fatal("no messages retrieved")
	}

	var msg map[string]string
	err = json.Unmarshal([]byte(msgs[0].Payload), &msg)
	if err != nil {
		t.Fatalf("failed to unmarshal message payload: %v", err)
	}

	if msg["msg"] != "Hello from test" {
		t.Errorf("message mismatch: got %v", msg)
	}
}

func TestConcurrencyConfig(t *testing.T) {
	tm := tenant.NewTenantManager(rabbitConn, pgPool, 5)
	tenantID := uuid.New()

	err := tm.CreateTenant(context.Background(), tenantID)
	if err != nil {
		t.Fatalf("failed to create tenant: %v", err)
	}

	// Update concurrency and ensure no panic or error
	err = tm.UpdateConcurrency(tenantID, 5)
	if err != nil {
		t.Fatalf("failed to update concurrency: %v", err)
	}

}
