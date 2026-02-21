package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"
)

type IdempotencyRecord struct {
	StatusCode int `json:"status_code"`
	Response   json.RawMessage `json:"response"`
	PayloadHash string `json:"payload_hash"`
	CreatedAt time.Time `json:"created_at"`
}

type IdempotencyStore interface {
	Get(ctx context.Context, key string) (*IdempotencyRecord, error)
	Set(ctx context.Context, key string, rec *IdempotencyRecord, ttl time.Duration) error
	Close() error
}

type RedisStore struct {
 	client *redis.Client
}
 
func NewRedisStore(addr string) *RedisStore {
 	rdb := redis.NewClient(&redis.Options{
  		Addr : addr,
    	Password: "",
     	DB: 0, 
  	})
  
  return &RedisStore{client : rdb}
}

func (s *RedisStore) Get(ctx context.Context, key string) (*IdempotencyRecord, error){
	data, err := s.client.Get(ctx, "idemp:"+key).Bytes()

	if err == redis.Nil {
		return nil, nil
	}
}