package main

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/lib/pq"
)

var (
	// ErrAlreadyExists is returned when item already exists in DB
	ErrAlreadyExists = errors.New("item already exists")
)

// Storage is responsible for CRUD operations with DB for news items
type Storage struct {
	db *sql.DB
}

func NewStorage(cfg Config) (*Storage, error) {
	db, err := openDB(cfg)
	if err != nil {
		return nil, err
	}

	return &Storage{db: db}, nil
}

// SaveItem saves news item to DB. Minimum required fields are Title and Link
func (s *Storage) SaveItem(ctx context.Context, item *NewsItem) error {

	args := []any{item.Title, item.Link}

	err := s.db.QueryRowContext(ctx,
		`INSERT INTO news (title, link) 
		VALUES ($1, $2)
		RETURNING id`,
		args...).Scan(&item.ID)

	if err != nil {
		pgErr, ok := err.(*pq.Error)
		// check if item already exists, return special error
		if ok && pgErr.Code == "23505" {
			return ErrAlreadyExists
		}
	}

	return err
}

// Close closes DB connection
func (s *Storage) Close() error {
	return s.db.Close()
}

// openDB opens connection to PostgreSQL DB, pings it and returns connection
func openDB(cfg Config) (*sql.DB, error) {
	db, err := sql.Open("postgres", cfg.DB.Dsn)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(cfg.DB.MaxOpenConns)
	db.SetMaxIdleConns(cfg.DB.MaxIdleConns)

	duration, err := time.ParseDuration(cfg.DB.MaxIdleTime)
	if err != nil {
		return nil, err
	}

	db.SetConnMaxIdleTime(duration)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = db.PingContext(ctx)
	if err != nil {
		return nil, err
	}

	return db, nil
}
