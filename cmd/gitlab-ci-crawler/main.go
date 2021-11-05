package main

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/ardanlabs/conf/v2"
	"github.com/deichindianer/gitlab-ci-crawler/internal/crawler"
)

var cfg crawler.Config

func init() {
	help, err := conf.Parse("", &cfg)
	if err != nil {
		if errors.Is(err, conf.ErrHelpWanted) {
			fmt.Println(help)
			return
		}
		log.Fatalf("parsing config: %s", err)
	}
}

func main() {
	c, err := crawler.New(cfg)
	if err != nil {
		log.Fatalf("failed to setup crawler: %s\n", err)
	}

	if err := c.Crawl(context.Background()); err != nil {
		log.Fatalf("failed to gather project data: %s", err)
	}
}
