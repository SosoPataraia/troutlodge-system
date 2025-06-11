package domain

import "context"

type OrderRepository interface {
	Create(ctx context.Context, order *Order) error
	Update(ctx context.Context, order *Order) error
	GetForUpdate(ctx context.Context, id string) (*Order, error)
	FindExpiredOrders(ctx context.Context) ([]*Order, error)
}
