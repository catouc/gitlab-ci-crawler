package main

import (
	"context"
	"github.com/deichindianer/gitlab-ci-crawler/internal/crawler"
	"github.com/deichindianer/gitlab-ci-crawler/internal/storage"
	"github.com/deichindianer/gitlab-ci-crawler/internal/storage/neo4j"
	"log"
)

var cfg crawler.Config
var neo4jcfg neo4j.Config

func main() {
	log.Println("configuring crawler...")
	if err := crawler.ParseConfig(&cfg); err != nil {
		log.Fatalf("failed to configure crawler: %s", err)
	}

	var err error
	var s storage.Storage
	switch cfg.Storage {
	case "neo4j":
		log.Println("configuring neo4j storage ...")
		s, err = neo4j.New(&neo4j.Config{
			Host:     neo4jcfg.Host,
			Username: neo4jcfg.Username,
			Password: neo4jcfg.Password,
			Realm:    "",
		})
		if err != nil {
			log.Fatalf("failed to configure neo4j storage: %s", err)
		}

		log.Println("successfully configured neo4j storage...")
	}

	c, err := crawler.New(&cfg, s)
	if err != nil {
		log.Fatalf("failed to setup crawler: %s\n", err)
	}
	log.Printf("successfully configured crawler with %s storage", cfg.Storage)

	if err := c.Crawl(context.Background()); err != nil {
		log.Fatalf("failed to gather project data: %s", err)
	}
}
