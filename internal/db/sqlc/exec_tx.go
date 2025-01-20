package db

import (
	"context"
	"fmt"
)

// ExecTx executes a function within a database transaction.
func (store *SQLStore) ExecTx(ctx context.Context, fn func(*Queries) error) error {
	tx, err := store.connPool.Begin(ctx)
	if err != nil {
		return err
	}
	
	qTx := New(tx)
	
	err = fn(qTx)
	if err != nil {
		rbErr := tx.Rollback(ctx)
		if rbErr != nil {
			return fmt.Errorf("transaction error: %v, rollback error: %v", err, rbErr)
		}
		
		return err
	}
	
	return tx.Commit(ctx)
}
