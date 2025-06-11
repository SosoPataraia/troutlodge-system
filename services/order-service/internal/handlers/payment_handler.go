// order-service/internal/handlers/payment.go
package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"troutlodge-system/services/order-service/internal/domain"
	"troutlodge-system/services/order-service/internal/repository"
	"troutlodge-system/shared/kafka"
)

type PaymentHandler struct {
	OrderRepo  domain.OrderRepository
	KafkaProd  *kafka.Producer
	DownpayCfg DownpaymentConfig
}

type DownpaymentConfig struct {
	Percent  float64
	Duration time.Duration
}

func (h *PaymentHandler) HandleDownpayment(w http.ResponseWriter, r *http.Request) {
	orderID := r.PathValue("id")

	// Get and lock order with distributed lock for scalability
	order, err := h.OrderRepo.GetForUpdate(r.Context(), orderID)
	if err != nil {
		http.Error(w, "Order not found", http.StatusNotFound)
		return
	}

	// Apply state transition
	if err := order.Reserve(h.DownpayCfg.Percent, h.DownpayCfg.Duration); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	// Optimistic locking update
	if err := h.OrderRepo.Update(r.Context(), order); err != nil {
		if err == repository.ErrOptimisticLock {
			http.Error(w, "Concurrent modification detected", http.StatusConflict)
			return
		}
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	// Publish event asynchronously
	go h.KafkaProd.Publish("order-reserved", map[string]interface{}{
		"order_id":   order.ID,
		"buyer_id":   order.BuyerID,
		"amount":     order.Downpayment,
		"expires_at": order.ExpiresAt.Format(time.RFC3339),
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(order)
}
