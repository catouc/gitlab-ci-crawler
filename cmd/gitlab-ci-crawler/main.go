package main

import (
	"context"

	"github.com/deichindianer/gitlab-ci-crawler/internal/crawler"
	"github.com/deichindianer/gitlab-ci-crawler/internal/storage"
	"github.com/deichindianer/gitlab-ci-crawler/internal/storage/neo4j"
	"github.com/rs/zerolog/log"
)

var cfg crawler.Config
var neo4jcfg neo4j.Config

func main() {
	log.Info().Msg("configuring crawler...")
	if err := crawler.ParseConfig(&cfg); err != nil {
		log.Fatal().Err(err).Msg("failed to configure crawler")
	}

	var err error
	var s storage.Storage
	switch cfg.Storage {
	case "neo4j":
		log.Info().Msg("configuring neo4j storage ...")
		s, err = neo4j.New(&neo4j.Config{
			Host:     neo4jcfg.Host,
			Username: neo4jcfg.Username,
			Password: neo4jcfg.Password,
			Realm:    "",
		})
		if err != nil {
			log.Fatal().Err(err).Msg("failed to configure neo4j storage")
		}

		log.Info().Msg("successfully configured neo4j storage...")
	}

	c, err := crawler.New(&cfg, s)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to setup crawler")
	}
	log.Info().Str("Storage", cfg.Storage).Msg("successfully configured crawler with storage")

	if err := c.Crawl(context.Background()); err != nil {
		log.Fatal().Err(err).Msg("failed to gather project data")
	}
}
