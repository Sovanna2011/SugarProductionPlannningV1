package database

import (
	"context"

	"gorm.io/gorm"
)

type ctxKey int

const txKey ctxKey = iota

// UnitOfWork owns transaction boundaries. Per §A2 rule 6 it is used from the
// service layer only; repositories never open or commit a transaction.
type UnitOfWork interface {
	// Do runs fn inside a transaction, committing on nil and rolling back on
	// error. A nested call joins the running transaction instead of opening a
	// second one, so a service may safely compose other services.
	Do(ctx context.Context, fn func(ctx context.Context) error) error
}

type unitOfWork struct {
	db *gorm.DB
}

func NewUnitOfWork(db *gorm.DB) UnitOfWork { return &unitOfWork{db: db} }

func (u *unitOfWork) Do(ctx context.Context, fn func(ctx context.Context) error) error {
	if _, ok := TxFrom(ctx); ok {
		return fn(ctx) // already inside a transaction — join it
	}
	return u.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(WithTx(ctx, tx))
	})
}

// WithTx stores a transaction handle in the context.
func WithTx(ctx context.Context, tx *gorm.DB) context.Context {
	return context.WithValue(ctx, txKey, tx)
}

// TxFrom returns the transaction handle carried by the context, if any.
func TxFrom(ctx context.Context) (*gorm.DB, bool) {
	tx, ok := ctx.Value(txKey).(*gorm.DB)
	return tx, ok
}

// Conn resolves the handle a repository should use: the ambient transaction
// when the caller opened one, otherwise the shared pool. Every repository
// method starts with this call, which is what makes a service able to compose
// repositories into one atomic operation (§D3).
func Conn(ctx context.Context, fallback *gorm.DB) *gorm.DB {
	if tx, ok := TxFrom(ctx); ok {
		return tx.WithContext(ctx)
	}
	return fallback.WithContext(ctx)
}
