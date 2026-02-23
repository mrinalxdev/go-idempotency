package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"terminologies/internal/middleware"
	"terminologies/internal/store"
	"time"

	"github.com/redis/go-redis/v9"
)

var ctx = context.Background()

const hmacSecret = "super-secret-key"

func createOrderHandler(
	idempStore store.IdempotencyStore,
	orderStore *store.OrderStore,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		key := r.Header.Get("Idempotency-Key")
		if key == "" {
			http.Error(w, "Idempotency-Key header required", http.StatusBadRequest)
			return
		}

		body, _ := io.ReadAll(r.Body)
		var req store.CreateOrderRequest
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		h := sha256.Sum256(body)
		payloadHash := hex.EncodeToString(h[:])
		rec, err := idempStore.Get(ctx, key)
		if err != nil {
			log.Printf("redis get error: %v", err)
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}

		if rec != nil {
			if rec.PayloadHash != payloadHash {
				http.Error(w, "Idempotency-Key reused with different payload", http.StatusConflict)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(rec.StatusCode)
			w.Write(rec.Response)
			log.Printf("idempotent replay key=%s", key)
			return
		}

		order, err := orderStore.CreateOrderIdempotent(ctx, req, key, http.StatusCreated, payloadHash)
		if err != nil {
			log.Printf("create order error: %v", err)
			http.Error(w, "Failed to create order", http.StatusInternalServerError)
			return
		}

		resp := struct {
			Order store.Order `json:"order"`
		}{*order}
		respBytes, _ := json.Marshal(resp)
		newRec := &store.IdempotencyRecord{
			StatusCode:  http.StatusCreated,
			Response:    respBytes,
			PayloadHash: payloadHash,
			CreatedAt:   time.Now().UTC(),
		}
		_ = idempStore.Set(ctx, key, newRec, 24*time.Hour)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write(respBytes)

		log.Printf("order created key=%s order=%s", key, order.ID)
	}
}

func main() {
	redisClient := redis.NewClient(&redis.Options{
		Addr: "localhost:6380",
	})

	redisStore := store.NewRedisStore("localhost:6380")

	orderStore, err := store.NewOrderStore(
		"postgres://demo:demo123@localhost:5433/idempotency?sslmode=disable",
	)
	if err != nil {
		log.Fatalf("postgres failed: %v", err)
	}
	defer redisStore.Close()
	defer orderStore.Close()

	if err := orderStore.Init(ctx); err != nil {
		log.Fatalf("table init failed: %v", err)
	}

	rateLimiter := middleware.NewRateLimiter(redisClient, 10, time.Minute)

	mux := http.NewServeMux()
	mux.HandleFunc("/orders", createOrderHandler(redisStore, orderStore))

	//chain: HMAC auth → rate limit → handler
	handler := middleware.HMACAuth(hmacSecret)(rateLimiter.Limit(mux))

	srv := &http.Server{Addr: ":8080", Handler: handler}

	go func() {
		log.Println("→ http://localhost:8080")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	shutdownCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	srv.Shutdown(shutdownCtx)
}