package api

import (
    "encoding/json"
    "net/http"
    "strconv"
	// "fmt"

    "github.com/google/uuid"
    "github.com/gorilla/mux"
    "github.com/srireskianita/multi-tenant-messaging/internal/tenant"
)

type MessageHandler struct {
    Manager *tenant.TenantManager
}
type MessageRequest struct {
    Payload string `json:"payload"`
}

// ListMessages godoc
// @Summary List messages with pagination
// @Description Mendapatkan daftar pesan tenant dengan cursor pagination
// @Tags messages
// @Produce json
// @Param tenant_id path string true "Tenant UUID"
// @Param cursor query string false "Cursor for pagination (message ID)"
// @Param limit query int false "Limit number of messages to return" default(10)
// @Success 200 {object} tenant.MessageListResponse
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /tenants/{tenant_id}/messages [get]
func (h *MessageHandler) ListMessages(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    tenantIDStr := vars["tenant_id"]
    cursor := r.URL.Query().Get("cursor")
    limitStr := r.URL.Query().Get("limit")

    tenantID, err := uuid.Parse(tenantIDStr)
    if err != nil {
        http.Error(w, `{"error":"invalid tenant_id format"}`, http.StatusBadRequest)
        return
    }

    limit := 10
    if limitStr != "" {
        if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
            limit = l
        }
    }

    messages, nextCursor, err := h.Manager.GetMessagesPaginated(r.Context(), tenantID.String(), cursor, limit)
    if err != nil {
        w.WriteHeader(http.StatusInternalServerError)
        json.NewEncoder(w).Encode(map[string]string{
            "error": err.Error(),
        })
        return
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]interface{}{
        "data":        messages,
        "next_cursor": nextCursor,
    })
}

func (h *MessageHandler) SendMessage(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    tenantIDStr := vars["tenant_id"]

    tenantID, err := uuid.Parse(tenantIDStr)
    if err != nil {
        http.Error(w, "invalid tenant_id", http.StatusBadRequest)
        return
    }

    var req MessageRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "invalid request body", http.StatusBadRequest)
        return
    }

    if err := h.Manager.PublishMessage(r.Context(), tenantID, req.Payload); err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    w.WriteHeader(http.StatusOK)
    w.Write([]byte("message sent successfully"))
}