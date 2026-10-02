package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

// Exhausting the pool pauses each operation inside database/sql acquisition,
// making the retirement boundary observable without relying on a lucky race.
func TestPoolRetirementWaitsForOperationStartup(t *testing.T) {
	operations := map[string]func(context.Context, *Store) error{
		"exec": func(ctx context.Context, s *Store) error { _, err := s.execContext(ctx, `SELECT 1`); return err },
		"query": func(ctx context.Context, s *Store) error {
			rows, err := s.queryContext(ctx, `SELECT 1`)
			if err != nil {
				return err
			}
			defer rows.Close()
			if !rows.Next() {
				return rows.Err()
			}
			var value int
			return rows.Scan(&value)
		},
		"queryRow": func(ctx context.Context, s *Store) error {
			var value int
			return s.queryRowContext(ctx, `SELECT 1`).Scan(&value)
		},
		"transaction": func(ctx context.Context, s *Store) error {
			return s.inTransaction(ctx, func(tx *Store) error { var value int; return tx.queryRowContext(ctx, `SELECT 1`).Scan(&value) })
		},
	}
	for name, operation := range operations {
		t.Run(name, func(t *testing.T) {
			db, err := openDB(filepath.Join(t.TempDir(), "index.sqlite"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			db.SetMaxOpenConns(1)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			held, err := db.Conn(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer held.Close()
			s := &Store{DB: db, live: &liveConn{}}
			done := make(chan error, 1)
			go func() { done <- operation(ctx, s) }()
			for db.Stats().WaitCount == 0 {
				select {
				case err := <-done:
					t.Fatalf("operation finished before waiting for a connection: %v", err)
				case <-ctx.Done():
					t.Fatal("operation never reached connection acquisition")
				case <-time.After(time.Millisecond):
				}
			}
			if s.live.mu.TryLock() {
				s.live.mu.Unlock()
				t.Fatal("pool retirement can proceed while an operation is acquiring its connection")
			}
			if err := held.Close(); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-ctx.Done():
				t.Fatal("operation did not finish after connection release")
			}
			if !s.live.mu.TryLock() {
				t.Fatal("operation retained lifecycle mutex after completion")
			}
			s.live.mu.Unlock()
		})
	}
}
