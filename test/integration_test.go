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
	// Start Docker pool
	pool, err := dockertest.NewPool("")
	if err != nil {
		log.Fatalf("Could not connect to Docker: %s", err)
	}

	// Start Postgres container
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

	// Build DSN
	dsn := fmt.Sprintf("postgres://test:secret@localhost:%s/testdb?sslmode=disable", resource.GetPort("5432/tcp"))

	// Retry until Postgres is ready (sql.Open + Ping)
	err = pool.Retry(func() error {
		var err error
		sqlDB, err = sql.Open("postgres", dsn)
		if err != nil {
			return err
		}
		return sqlDB.Ping()
	})
	if err != nil {
		log.Fatalf("Could not connect to database: %s", err)
	}

	// Run migration
	if err := runMigrations(sqlDB); err != nil {
		log.Fatalf("Could not run migrations: %s", err)
	}

	// Setup pgxpool for TenantManager
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		log.Fatalf("Failed to parse pgxpool config: %v", err)
	}
	pgPool, err = pgxpool.ConnectConfig(context.Background(), config)
	if err != nil {
		log.Fatalf("Failed to connect pgxpool: %v", err)
	}

	// Connect to RabbitMQ (pastikan RabbitMQ sudah berjalan di localhost:5672)
	rabbitConn, err = amqp091.Dial("amqp://guest:guest@localhost:5672/")
	if err != nil {
		log.Fatalf("Failed to connect RabbitMQ: %v", err)
	}

	// Run tests
	code := m.Run()

	// Cleanup
	rabbitConn.Close()
	pgPool.Close()
	_ = pool.Purge(resource)
	os.Exit(code)
}

func runMigrations(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS messages (
		id UUID PRIMARY KEY,
		tenant_id UUID NOT NULL,
		payload JSONB,
		created_at TIMESTAMPTZ DEFAULT NOW()
	) PARTITION BY LIST (tenant_id);
	`
	_, err := db.Exec(schema)
	return err
}

func TestTenantLifecycle(t *testing.T) {
	ctx := context.Background()
	tm := tenant.NewTenantManager(rabbitConn, pgPool, 5)

	tenantID := uuid.New()
	err := tm.CreateTenant(ctx, tenantID)
	if err != nil {
		t.Fatalf("failed to create tenant: %v", err)
	}

	// Asumsi ada GetTenant method (sesuaikan jika beda)
	tenantData, err := tm.GetTenant(ctx, tenantID)
	if err != nil || tenantData == nil {
		t.Fatalf("tenant not found after creation")
	}

	err = tm.DeleteTenant(ctx, tenantID)
	if err != nil {
		t.Fatalf("failed to delete tenant: %v", err)
	}

	tenantData, err = tm.GetTenant(ctx, tenantID)
	if err == nil && tenantData != nil {
		t.Fatalf("tenant was not deleted")
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

	time.Sleep(2 * time.Second) // tunggu pesan diproses (kalau ada async)

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

	err = tm.UpdateConcurrency(tenantID, 5)
	if err != nil {
		t.Fatalf("failed to update concurrency: %v", err)
	}

	// Jika ada method GetWorkerCount
	workers := tm.GetWorkerCount(tenantID)
	if workers != 5 {
		t.Errorf("expected 5 workers, got %d", workers)
	}
}
