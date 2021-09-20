package main

import (
	"log"
	"os"

	"github.com/deichindianer/dependency-seeker/internal/projects"
)

func main() {
	s, err := projects.New("https://test.source.tui", os.Getenv("GITLAB_TOKEN"))
	if err != nil {
		log.Fatalf("failed to setup source: %s\n", err)
	}

	err = s.GatherProjectData(projects.GatherProjectDataInput{
		OutPath: "out",
		GroupID: 3788,
	})
	if err != nil {
		log.Fatalf("failed to gather project data: %s", err)
	}
}
