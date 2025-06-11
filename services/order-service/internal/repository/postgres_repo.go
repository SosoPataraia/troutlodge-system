// order-service/internal/repository/postgres.go
package repository

import (
	"context"
	"database/sql"
	"errors"
	"troutlodge-system/services/order-service/internal/domain"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
)

var ErrOptimisticLock = errors.New("optimistic locking failed")

type PostgresOrderRepo struct {
	db *sql.DB
}

func NewPostgresOrderRepo(connStr string) (*PostgresOrderRepo, error) {
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}
	return &PostgresOrderRepo{db: db}, nil
}

func (r *PostgresOrderRepo) Create(ctx context.Context, order *domain.Order) error {
	query := `INSERT INTO orders (id, buyer_id, egg_type, quantity, state, expires_at, version, downpayment, total_amount) 
	          VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`
	_, err := r.db.ExecContext(ctx, query,
		uuid.New().String(),
		order.BuyerID,
		order.EggType,
		order.Quantity,
		order.State,
		order.ExpiresAt,
		1, // Initial version
		order.Downpayment,
		order.TotalAmount,
	)
	return err
}

func (r *PostgresOrderRepo) Update(ctx context.Context, order *domain.Order) error {
	query := `UPDATE orders 
	          SET state = $1, downpayment = $2, version = version + 1, expires_at = $3
	          WHERE id = $4 AND version = $5`
	result, err := r.db.ExecContext(ctx, query,
		order.State,
		order.Downpayment,
		order.ExpiresAt,
		order.ID,
		order.Version,
	)
	if err != nil {
		return err
	}
	if rowsAffected, _ := result.RowsAffected(); rowsAffected == 0 {
		return ErrOptimisticLock
	}
	return nil
}

func (r *PostgresOrderRepo) GetForUpdate(ctx context.Context, id string) (*domain.Order, error) {
	query := `SELECT id, buyer_id, egg_type, quantity, state, expires_at, version, downpayment, total_amount
              FROM orders
              WHERE id = $1
              FOR UPDATE SKIP LOCKED` // Critical for scalability

	row := r.db.QueryRowContext(ctx, query, id)
	order := &domain.Order{}
	err := row.Scan(
		&order.ID,
		&order.BuyerID,
		&order.EggType,
		&order.Quantity,
		&order.State,
		&order.ExpiresAt,
		&order.Version,
		&order.Downpayment,
		&order.TotalAmount,
	)
	if err != nil {
		return nil, err
	}
	return order, nil
}

func (r *PostgresOrderRepo) FindExpiredOrders(ctx context.Context) ([]*domain.Order, error) {
	query := `SELECT id, buyer_id, egg_type, quantity, state, expires_at, version, downpayment, total_amount 
	          FROM orders WHERE state = $1 AND expires_at < NOW() LIMIT 100` // Batch processing for scalability

	rows, err := r.db.QueryContext(ctx, query, domain.Reserved)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orders []*domain.Order
	for rows.Next() {
		var o domain.Order
		err := rows.Scan(&o.ID, &o.BuyerID, &o.EggType, &o.Quantity, &o.State, &o.ExpiresAt, &o.Version, &o.Downpayment, &o.TotalAmount)
		if err != nil {
			return nil, err
		}
		orders = append(orders, &o)
	}
	return orders, nil
}
