package main

import (
    "context"
    "log"
    "net/http"
    "os"
    "os/signal"
    "syscall"
    "time"

    "github.com/gorilla/mux"
    httpSwagger "github.com/swaggo/http-swagger"
    _ "github.com/srireskianita/multi-tenant-messaging/docs"

    "github.com/srireskianita/multi-tenant-messaging/internal/config"
    "github.com/srireskianita/multi-tenant-messaging/internal/db"
    "github.com/srireskianita/multi-tenant-messaging/internal/queue"
    "github.com/srireskianita/multi-tenant-messaging/internal/api"
    "github.com/srireskianita/multi-tenant-messaging/internal/tenant"
)

func main() {
    cfg := config.Load()

    db.Init(cfg.Database.URL)
    queue.Init(cfg.RabbitMQ.URL)

    tenantManager := tenant.NewTenantManager(queue.Conn, db.Pool, cfg.Workers)

    tenantHandler := &api.TenantHandler{Manager: tenantManager}
    messageHandler := &api.MessageHandler{Manager: tenantManager}

    r := mux.NewRouter()
    r.HandleFunc("/tenants", tenantHandler.CreateTenant).Methods("POST")
    r.HandleFunc("/tenants/{id}", tenantHandler.DeleteTenant).Methods("DELETE")
    r.HandleFunc("/tenants/{id}/config/concurrency", tenantHandler.UpdateConcurrency).Methods("PUT")

    r.HandleFunc("/tenants/{tenant_id}/messages", messageHandler.ListMessages).Methods("GET")
    r.HandleFunc("/tenants/{tenant_id}/messages", messageHandler.SendMessage).Methods("POST")

    r.PathPrefix("/swagger/").Handler(httpSwagger.WrapHandler)

	srv := &http.Server{
		Addr:    ":8080",
		Handler: r,
	}

    go func() {
        log.Println("🚀 Server running on :8080")
        if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            log.Fatalf("❌ Listen error: %v", err)
        }
    }()

    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    <-quit
    log.Println("🛑 Shutting down server...")

    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    if err := srv.Shutdown(ctx); err != nil {
        log.Fatalf("❌ Server forced to shutdown: %v", err)
    }

    log.Println("📦 Closing DB and RabbitMQ connections...")
    db.Pool.Close()
    queue.Conn.Close()

    log.Println("✅ Server exited gracefully")
}