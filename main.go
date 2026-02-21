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
	"time"

	"terminologies/internal/store"
	// "github.com/google/uuid"
)

var ctx = context.Background()

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

        var req store.CreateOrderRequest
        body, _ := io.ReadAll(r.Body)
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

        order, err := orderStore.CreateOrder(ctx, req)
        if err != nil {
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
    redisStore := store.NewRedisStore("localhost:6380")
    orderStore, err := store.NewOrderStore("postgres://demo:demo123@localhost:5433/idempotency?sslmode=disable")
    if err != nil {
        log.Fatalf("postgres failed: %v", err)
    }
    defer redisStore.Close()
    defer orderStore.Close()


    if err := orderStore.Init(ctx); err != nil {
      log.Printf("warning: table init failed: %v", err)
    }

    mux := http.NewServeMux()
    mux.HandleFunc("/orders", createOrderHandler(redisStore, orderStore))

    srv := &http.Server{Addr: ":8080", Handler: mux}

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