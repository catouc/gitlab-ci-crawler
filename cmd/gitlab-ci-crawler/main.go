package main

import (
	"context"
	"os"

	"github.com/catouc/gitlab-ci-crawler/internal/crawler"
	"github.com/catouc/gitlab-ci-crawler/internal/storage"
	"github.com/catouc/gitlab-ci-crawler/internal/storage/neo4j"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var cfg crawler.Config
var neo4jcfg neo4j.Config

func init() {
	if err := crawler.ParseConfig(&cfg); err != nil {
		log.Fatal().Err(err).Msg("failed to parse crawler config")
	}

	switch cfg.LogFormat {
	case "text":
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout})
	case "json":
		log.Logger = log.Output(os.Stdout)
	default:
		log.Fatal().
			Str("LogFormat", cfg.LogFormat).
			Msg("unsupported log format")
	}

	zerolog.SetGlobalLevel(zerolog.Level(cfg.LogLevel))

	log.Info().
		Str("GitlabHost", cfg.GitlabHost).
		Int("GitLabMaxRPS", cfg.GitlabMaxRPS).
		Msg("configured crawler")
}

func main() {
	rootCtx, rootCancel := context.WithCancel(context.Background())
	defer rootCancel()

	storageLogger := log.With().Str("StorageType", cfg.Storage).Logger()

	storageLogger.Info().Msg("configuring storage...")
	var err error
	var s storage.Storage
	switch cfg.Storage {
	case "neo4j":
		s, err = neo4j.New(&neo4jcfg)
		if err != nil {
			storageLogger.Fatal().Err(err).Msg("failed to configure storage")
		}

		storageLogger.Info().
			Str("Host", neo4jcfg.Host).
			Str("Username", neo4jcfg.Username).
			Msg("successfully configured storage")
	default:
		storageLogger.Fatal().Msgf("unknown storage: %s", cfg.Storage)
	}

	c, err := crawler.New(&cfg, log.Logger, s)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to setup crawler")
	}
	log.Info().Str("Storage", cfg.Storage).Msg("successfully configured crawler with storage")

	if err := c.Crawl(rootCtx); err != nil {
		log.Fatal().Err(err).Msg("failed to gather project data")
	}
}
