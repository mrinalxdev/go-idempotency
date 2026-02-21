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

	var rec IdempotencyRecord

	if err := json.Unmarshal(data, &rec); err != nil {
		return nil, err
	}

	return &rec, nil
}


func (s *RedisStore) Set(ctx context.Context, key string, rec *IdempotencyRecord, ttl time.Duration) error {
	data, err := json.Marshal(rec)
	if err != nil {
		return err
	}

	return s.client.Set(ctx, "idemp:"+key, data, ttl).Err()
}

func (s *RedisStore) Close() error {
	return s.client.Close()
}