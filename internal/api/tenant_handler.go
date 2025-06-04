package api

import (
	"encoding/json"
	"net/http"
	// "strconv"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/srireskianita/multi-tenant-messaging/internal/tenant"
)

// TenantRequest represents the tenant creation request body
type TenantRequest struct {
	ID string `json:"id"` // tenant id from client
}

// ConcurrencyRequest represents the concurrency update request body
type ConcurrencyRequest struct {
	Workers int `json:"workers"`
}

// MessageResponse represents a generic JSON response message
type MessageResponse struct {
	Message string `json:"message"`
}

type TenantHandler struct {
	Manager *tenant.TenantManager
}

// CreateTenant godoc
// @Summary Create new tenant
// @Description Membuat tenant baru dan memulai konsumer
// @Tags tenants
// @Accept json
// @Produce json
// @Param tenant body TenantRequest true "Tenant info"
// @Success 201 {object} MessageResponse
// @Failure 400 {object} MessageResponse
// @Failure 500 {object} MessageResponse
// @Router /tenants [post]
func (h *TenantHandler) CreateTenant(w http.ResponseWriter, r *http.Request) {
	var req TenantRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, MessageResponse{"invalid request body"})
		return
	}

	tenantID, err := uuid.Parse(req.ID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, MessageResponse{"invalid tenant id"})
		return
	}

	err = h.Manager.CreateTenant(r.Context(), tenantID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, MessageResponse{"failed to create tenant: " + err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, MessageResponse{"tenant created"})
}

// DeleteTenant godoc
// @Summary Delete tenant
// @Description Menghapus tenant dan menghentikan konsumer
// @Tags tenants
// @Produce json
// @Param id path string true "Tenant UUID"
// @Success 200 {object} MessageResponse
// @Failure 400 {object} MessageResponse
// @Failure 500 {object} MessageResponse
// @Router /tenants/{id} [delete]
func (h *TenantHandler) DeleteTenant(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr := vars["id"]

	tenantID, err := uuid.Parse(idStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, MessageResponse{"invalid tenant id"})
		return
	}

	err = h.Manager.DeleteTenant(r.Context(), tenantID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, MessageResponse{"failed to delete tenant: " + err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, MessageResponse{"tenant deleted"})
}

// UpdateConcurrency godoc
// @Summary Update concurrency config
// @Description Update jumlah worker konsumer untuk tenant
// @Tags tenants
// @Accept json
// @Produce json
// @Param id path string true "Tenant UUID"
// @Param concurrency body ConcurrencyRequest true "Concurrency config"
// @Success 200 {object} MessageResponse
// @Failure 400 {object} MessageResponse
// @Failure 500 {object} MessageResponse
// @Router /tenants/{id}/config/concurrency [put]
func (h *TenantHandler) UpdateConcurrency(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr := vars["id"]

	tenantID, err := uuid.Parse(idStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, MessageResponse{"invalid tenant id"})
		return
	}

	var req ConcurrencyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Workers <= 0 {
		writeJSON(w, http.StatusBadRequest, MessageResponse{"invalid workers count"})
		return
	}

	// Gunakan method yang benar: UpdateConcurrency
	err = h.Manager.UpdateConcurrency(tenantID, req.Workers)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, MessageResponse{"failed to update concurrency: " + err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, MessageResponse{"concurrency updated"})
}

// writeJSON helper for consistent JSON response
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}