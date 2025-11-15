package database

import (
	"context"
	"time"
)

// Timeout constants for different types of database operations
const (
	// DefaultQueryTimeout for simple SELECT queries
	DefaultQueryTimeout = 5 * time.Second

	// ComplexQueryTimeout for complex queries with JOINs or aggregations
	ComplexQueryTimeout = 10 * time.Second

	// WriteTimeout for INSERT/UPDATE/DELETE operations
	WriteTimeout = 8 * time.Second

	// TransactionTimeout for multi-statement transactions
	TransactionTimeout = 15 * time.Second
)

// NewQueryContext creates a context with default timeout for simple queries
func NewQueryContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), DefaultQueryTimeout)
}

// NewComplexQueryContext creates a context with extended timeout for complex queries
func NewComplexQueryContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), ComplexQueryTimeout)
}

// NewWriteContext creates a context with timeout for write operations
func NewWriteContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), WriteTimeout)
}

// NewTransactionContext creates a context with timeout for transactions
func NewTransactionContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), TransactionTimeout)
}

// NewCustomContext creates a context with custom timeout
func NewCustomContext(timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), timeout)
}
