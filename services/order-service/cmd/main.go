package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"troutlodge-system/services/order-service/internal/domain"
	"troutlodge-system/services/order-service/internal/handlers"
	"troutlodge-system/services/order-service/internal/repository"
	"troutlodge-system/shared/kafka"

	"github.com/redis/go-redis/v9"
)

func main() {
	// Initialize Redis
	rdb := redis.NewClient(&redis.Options{
		Addr:     "redis:6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	// Initialize dependencies
	connStr := "postgres://admin:securepass@postgres:5432/troutlodge?sslmode=disable"
	pgRepo, err := repository.NewPostgresOrderRepo(connStr)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}

	// Create cached repository
	cachedRepo := repository.NewCachedOrderRepository(pgRepo, rdb, 5*time.Minute)

	// Initialize Kafka producer
	kafkaProd := kafka.NewProducer("kafka:9092")

	// Create payment handler with cached repository
	paymentHandler := &handlers.PaymentHandler{
		OrderRepo: cachedRepo, // Using the cached repository
		KafkaProd: kafkaProd,
		DownpayCfg: handlers.DownpaymentConfig{
			Percent:  0.15,
			Duration: 48 * time.Hour,
		},
	}

	// Setup HTTP server with graceful shutdown
	server := &http.Server{
		Addr:    ":8080",
		Handler: setupRoutes(paymentHandler),
	}

	// Start background processors
	go startOrderExpirationChecker(cachedRepo, kafkaProd) // Use cachedRepo here too

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Println("Starting order service on :8080")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	<-quit
	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	defer kafkaProd.Close()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server shutdown failed: %v", err)
	}
	log.Println("Server exited properly")
}

func setupRoutes(ph *handlers.PaymentHandler) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /orders/{id}/reserve", ph.HandleDownpayment)

	// Add health check endpoint for Kubernetes
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	return mux
}

func startOrderExpirationChecker(repo domain.OrderRepository, kafkaProd *kafka.Producer) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			expiredOrders, err := repo.FindExpiredOrders(ctx)
			cancel()

			if err != nil {
				log.Printf("Error finding expired orders: %v", err)
				continue
			}

			for _, order := range expiredOrders {
				if err := order.Cancel(); err != nil {
					log.Printf("Error cancelling order %s: %v", order.ID, err)
					continue
				}

				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				if err := repo.Update(ctx, order); err != nil {
					log.Printf("Error updating order %s: %v", order.ID, err)
					cancel()
					continue
				}
				cancel()

				kafkaProd.Publish("order-cancelled", map[string]interface{}{
					"order_id": order.ID,
					"reason":   "downpayment_timeout",
				})
			}
		}
	}
}
