package store

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type CreateOrderRequest struct {
	UserID    string `json:"user_id"`
	ProductID string `json:"product_id"`
	Quantity  int    `json:"quantity"`
}

type Order struct {
	ID        string    `json:"order_id"`
	UserID    string    `json:"user_id"`
	ProductID string    `json:"product_id"`
	Quantity  int       `json:"quantity"`
	CreatedAt time.Time `json:"created_at"`
}

type OrderStore struct {
	db *pgxpool.Pool
}

func NewOrderStore(connString string) (*OrderStore, error){
	pool, err := pgxpool.New(context.Background(), connString)

	if err != nil {
		return nil, err
	}

	return &OrderStore{db:pool}, nil
}

func (s *OrderStore) Init(ctx context.Context) error {
    _, err := s.db.Exec(ctx, `
        CREATE TABLE IF NOT EXISTS orders (
            id          TEXT PRIMARY KEY,
            user_id     TEXT NOT NULL,
            product_id  TEXT NOT NULL,
            quantity    INTEGER NOT NULL,
            created_at  TIMESTAMPTZ NOT NULL
        );
    `)
    return err
}


func (s *OrderStore) CreateOrder(ctx context.Context, req CreateOrderRequest) (*Order, error) {
	order := &Order {
		ID:        "ord_" + uuid.NewString()[:10],
        UserID:    req.UserID,
        ProductID: req.ProductID,
        Quantity:  req.Quantity,
        CreatedAt: time.Now().UTC(),
	}

	_, err := s.db.Exec(ctx,
        `INSERT INTO orders (id, user_id, product_id, quantity, created_at)
         VALUES ($1, $2, $3, $4, $5)`,
        order.ID, order.UserID, order.ProductID, order.Quantity, order.CreatedAt,
    )
    if err != nil {
        return nil, err
    }

    return order, nil
}

func (s *OrderStore) Close() {
	s.db.Close()
}