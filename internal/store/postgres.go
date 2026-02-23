package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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

func NewOrderStore(connString string) (*OrderStore, error) {
	pool, err := pgxpool.New(context.Background(), connString)
	if err != nil {
		return nil, err
	}
	return &OrderStore{db: pool}, nil
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
		CREATE TABLE IF NOT EXISTS idempotency_records (
			key          TEXT PRIMARY KEY,
			status_code  INTEGER NOT NULL,
			response     JSONB NOT NULL,
			payload_hash TEXT NOT NULL,
			created_at   TIMESTAMPTZ NOT NULL
		);
	`)
	return err
}


func (s *OrderStore) CreateOrderIdempotent(
	ctx context.Context,
	req CreateOrderRequest,
	idempKey string,
	statusCode int,
	payloadHash string,
) (*Order, error) {
	order := &Order{
		ID:        "ord_" + uuid.NewString()[:10],
		UserID:    req.UserID,
		ProductID: req.ProductID,
		Quantity:  req.Quantity,
		CreatedAt: time.Now().UTC(),
	}

	err := pgx.BeginTxFunc(ctx, s.db, pgx.TxOptions{}, func(tx pgx.Tx) error {
		
		_, err := tx.Exec(ctx,
			`INSERT INTO orders (id, user_id, product_id, quantity, created_at)
			 VALUES ($1, $2, $3, $4, $5)`,
			order.ID, order.UserID, order.ProductID, order.Quantity, order.CreatedAt,
		)
		if err != nil {
			return err
		}

	
		resp := struct {
			Order Order `json:"order"`
		}{*order}
		respBytes, err := json.Marshal(resp)
		if err != nil {
			return err
		}

		_, err = tx.Exec(ctx,
			`INSERT INTO idempotency_records (key, status_code, response, payload_hash, created_at)
			 VALUES ($1, $2, $3, $4, $5)
			 ON CONFLICT (key) DO NOTHING`,
			idempKey, statusCode, respBytes, payloadHash, time.Now().UTC(),
		)
		return err
	})

	if err != nil {
		return nil, err
	}
	return order, nil
}

func (s *OrderStore) Close() {
	s.db.Close()
}