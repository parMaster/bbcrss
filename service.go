package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"
)

type Service struct {
	cfg     *Config
	Parser  *Parser
	Storage *Storage
}

func NewService(cfg *Config) (*Service, error) {

	parser := NewParser(cfg)

	storage, err := NewStorage(*cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to start storage: %w", err)
	}

	return &Service{
		cfg:     cfg,
		Parser:  parser,
		Storage: storage,
	}, nil
}

// ParsingJob runs parsing job with given interval, saves items to DB and publishes to the queue
func (s *Service) ParsingJob(ctx context.Context) {
	log.Println("starting parsing job ...")

	ttl, err := time.ParseDuration(s.cfg.RssTtl)
	if err != nil {
		log.Println("failed to parse RSS TTL, using default 15m")
		ttl = 15 * time.Minute
	}

	ticker := time.NewTicker(ttl)
	retry, limit := 0, 3
	for {
		log.Println("parsing RSS feed")
		items, err := s.Parser.ParseRSS(ctx)
		if err != nil {
			log.Printf("failed to parse RSS: %v, retrying in 30 sec %d/%d", err, retry, limit)
			retry++
			if retry > limit {
				log.Printf("failed to parse RSS: %v, exiting", err)
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(30 * time.Second):
				continue
			}
		}
		retry = 0
		log.Printf("parsed %d items", len(items))

		// Saving items to DB
		saved, skipped := 0, 0
		for _, item := range items {
			err := s.Storage.SaveItem(ctx, &item)
			if err != nil {
				if errors.Is(err, ErrAlreadyExists) {
					log.Printf("[DEBUG] item already exists: %v", item)
					skipped++
					continue
				}
				log.Printf("[ERROR] failed to save item: %v", err)
				continue
			}
			saved++

			log.Printf("[INFO] %d news saved, %d duplicates skipped", saved, skipped)
			log.Printf("[DEBUG] item saved: %v", item)

			// TODO: publish items to the queue
		}

		select {
		case <-ticker.C:
			// ttl expired, parse again
			continue
		case <-ctx.Done():
			log.Printf("parsing job stopped: %v", ctx.Err())
			return
		}
	}
}

// Run starts the service and waits for termination signal
// Parsing and enrichment job runs in background
func (s *Service) Run(ctx context.Context) {

	go s.ParsingJob(ctx)

	// TODO: add enrichment job

	// wait for termination signal
	<-ctx.Done()
	err := s.Storage.Close()
	if err != nil {
		log.Printf("failed to close storage: %v", err)
	}
}
