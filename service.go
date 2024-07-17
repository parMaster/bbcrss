package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"
)

type Service struct {
	cfg       *Config
	Parser    *Parser
	Storage   *Storage
	ApiServer *APIServer
	Mq        *Mq
}

func NewService(cfg *Config) (*Service, error) {

	parser := NewParser(cfg)

	storage, err := NewStorage(cfg.DB)
	if err != nil {
		return nil, fmt.Errorf("failed to start storage: %w", err)
	}

	mq, err := NewMq(cfg.RMQ)
	if err != nil {
		return nil, fmt.Errorf("failed to start RabbitMQ: %w", err)
	}

	api, err := NewAPIServer(storage, cfg.API)
	if err != nil {
		return nil, fmt.Errorf("failed to start API server: %w", err)
	}

	return &Service{
		cfg:       cfg,
		Parser:    parser,
		Storage:   storage,
		Mq:        mq,
		ApiServer: api,
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
		items, err := s.Parser.GetNews(ctx)
		if err != nil {
			if retry > limit {
				log.Printf("[ERROR] failed to parse RSS: %v, exiting", err)
				return
			}
			log.Printf("failed to parse RSS: %v, retrying in 30 sec %d/%d", err, retry, limit)
			retry++
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
			err := s.Storage.CreateNewsItem(ctx, &item)
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

			// log.Printf("[DEBUG] item saved: %v", item)

			// publish item link to the queue
			err = s.Mq.Publish([]byte(item.Link))
			if err != nil {
				log.Printf("[ERROR] failed to publish to queue: %v", err)
			}
		}
		log.Printf("[INFO] %d news saved, %d duplicates skipped", saved, skipped)

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

// EnrichmentJob consumes links from the queue, gets news item from DB, enriches it and saves back
func (s *Service) EnrichmentJob(ctx context.Context) {
	newsCh, err := s.Mq.Consume()
	if err != nil {
		log.Fatalf("failed to consume messages: %v", err)
	}
	log.Println("starting enrichment job ...")

	for msg := range newsCh {
		link := string(msg.Body)
		log.Printf("[DEBUG] enriching news: %s", link)

		err := s.EnrichNewsItem(ctx, link)
		if err != nil {
			log.Printf("[ERROR] failed to enrich news: %v", err)
		}
	}
}

// EnrichNewsItem enriches news item with additional data
func (s *Service) EnrichNewsItem(ctx context.Context, link string) error {
	// get news item from DB
	newsItem, err := s.Storage.GetNewsItem(ctx, link)
	if err != nil {
		log.Printf("[ERROR] failed to get item from DB: %v", err)
		return fmt.Errorf("failed to get item from DB: %w", err)
	}

	// enrich news item
	applied, err := s.Parser.Enrich(ctx, newsItem)
	if err != nil {
		log.Printf("failed to enrich news: %v", err)
		return fmt.Errorf("failed to enrich news: %w", err)
	}
	log.Printf("[DEBUG] %d enrichments applied to id=%d", applied, newsItem.ID)

	// save enriched news item
	err = s.Storage.SaveNewsItem(ctx, newsItem)
	if err != nil {
		log.Printf("[ERROR] failed to save item: %v", err)
		return fmt.Errorf("failed to save item: %w", err)
	}

	log.Printf("[DEBUG] item saved: %v", newsItem)
	return nil
}

// Run starts the service and waits for termination signal
// Parsing and Enrichment jobs run in background
func (s *Service) Run(ctx context.Context) {

	go s.ParsingJob(ctx)

	go s.EnrichmentJob(ctx)

	go func() {
		err := s.ApiServer.Run(ctx)
		if err != nil {
			log.Printf("failed to start API server: %v", err)
			return
		}
	}()

	// wait for termination signal
	<-ctx.Done()

	err := s.Mq.Close()
	if err != nil {
		log.Printf("failed to close RabbitMQ: %v", err)
	}

	err = s.Storage.Close()
	if err != nil {
		log.Printf("failed to close storage: %v", err)
	}
}
