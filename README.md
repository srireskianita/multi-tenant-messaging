Multi-Tenant Messaging System

## Overview
A Go application that manages multi-tenant messaging using RabbitMQ and PostgreSQL.  
It supports dynamic consumer management per tenant, partitioned data storage, configurable concurrency, cursor pagination, and graceful shutdown. The system includes APIs, Swagger documentation, and automated integration tests.

---

## Features

- **Auto-spawn and auto-stop tenant RabbitMQ consumers** on tenant creation/deletion
- Partitioned PostgreSQL table for message storage by tenant
- Configurable worker pool concurrency per tenant via API
- Cursor-based pagination API for message retrieval
- Graceful shutdown ensuring ongoing transactions are processed before stopping
- Swagger/OpenAPI documentation for all endpoints
- Integration tests with Dockerized PostgreSQL and RabbitMQ using Dockertest
- Configuration via YAML file (RabbitMQ, database, default workers)
- JWT authentication for tenant operations
- Retry logic with dead-letter queues for failed messages
- Prometheus metrics monitoring for queue depth and worker status

---

## Getting Started

### Prerequisites

- Go 1.18 or higher
- Docker (for running RabbitMQ and PostgreSQL during development and tests)
- RabbitMQ and PostgreSQL instances (can run locally or via Docker)

---

### Installation

Clone the repo:

git clone https://github.com/username/multi-tenant-messaging.git
cd multi-tenant-messaging

Install dependencies:
go mod tidy
Configure your environment variables or YAML config file (config.yaml), example:

yaml
rabbitmq:
  url: amqp://user:pass@localhost:5672/
database:
  url: postgres://user:pass@localhost:5432/app
workers: 3

Running the Application
go run main.go

Or build and run:
go build -o app
./app

API Endpoints
POST /tenants
Create tenant, auto-spawn dedicated RabbitMQ consumer and queue

DELETE /tenants/{id}
Delete tenant, stop consumer, and remove queue

PUT /tenants/{id}/config/concurrency
Update concurrency workers count
Request body:
{ "workers": 5 }
GET /messages?cursor={cursor}

Cursor pagination for messages, returns:
{
  "data": [ ... ],
  "next_cursor": "..."
}

Swagger UI:
Available at /swagger/index.html

Database Schema
PostgreSQL messages table partitioned by tenant_id:
CREATE TABLE messages (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL,
  payload JSONB,
  created_at TIMESTAMPTZ DEFAULT NOW()
) PARTITION BY LIST (tenant_id);

Testing
Run integration tests with Dockertest-managed PostgreSQL and RabbitMQ:
go test ./... -v

Tests cover:

Tenant lifecycle (creation and destruction)

Message publishing and consumption

Worker concurrency updates

Graceful Shutdown
The app ensures in-flight messages are processed before shutting down consumers and the app itself.

Configuration
Use YAML config or environment variables to manage:

RabbitMQ connection

PostgreSQL connection

Default worker concurrency

Monitoring and Security
Prometheus metrics endpoint to monitor queue depth and worker status

JWT-based authentication for tenant-specific APIs

Retry logic and dead-letter queues for failed messages

Contributing 
Contributions are welcome. Please fork the repo and submit pull requests or open issues for bugs and feature requests.

License
This project is licensed under the MIT License.

Contact
Sri Reski Anita - srireskianita@gmail.com
Project Link: https://github.com/srireskianita/multi-tenant-messaging
