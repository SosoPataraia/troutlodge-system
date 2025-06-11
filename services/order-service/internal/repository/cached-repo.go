package repository

import (
	"context"
	"encoding/json"
	"time"

	"troutlodge-system/services/order-service/internal/domain"

	"github.com/redis/go-redis/v9"
)

type CachedOrderRepository struct {
	primaryRepo domain.OrderRepository
	redisClient *redis.Client
	ttl         time.Duration
}

func NewCachedOrderRepository(
	primary domain.OrderRepository,
	redisClient *redis.Client,
	cacheTTL time.Duration,
) *CachedOrderRepository {
	return &CachedOrderRepository{
		primaryRepo: primary,
		redisClient: redisClient,
		ttl:         cacheTTL,
	}
}

func (r *CachedOrderRepository) GetForUpdate(ctx context.Context, id string) (*domain.Order, error) {
	cacheKey := "order:" + id

	// Try cache first
	cached, err := r.redisClient.Get(ctx, cacheKey).Bytes()
	if err == nil {
		var order domain.Order
		if err := json.Unmarshal(cached, &order); err == nil {
			return &order, nil
		}
	}

	// Fallback to primary repository
	order, err := r.primaryRepo.GetForUpdate(ctx, id)
	if err != nil {
		return nil, err
	}

	// Update cache
	if order != nil {
		data, _ := json.Marshal(order)
		r.redisClient.Set(ctx, cacheKey, data, r.ttl)
	}

	return order, nil
}

// Implement other methods by delegating to primaryRepo
func (r *CachedOrderRepository) Create(ctx context.Context, order *domain.Order) error {
	return r.primaryRepo.Create(ctx, order)
}

func (r *CachedOrderRepository) Update(ctx context.Context, order *domain.Order) error {
	// Invalidate cache on update
	defer r.redisClient.Del(ctx, "order:"+order.ID)
	return r.primaryRepo.Update(ctx, order)
}

func (r *CachedOrderRepository) FindExpiredOrders(ctx context.Context) ([]*domain.Order, error) {
	return r.primaryRepo.FindExpiredOrders(ctx)
}
