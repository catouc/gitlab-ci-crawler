package main

import (
	"log"
	"os"

	"github.com/deichindianer/dependency-seeker/internal/projects"
)


func main() {
	if os.Getenv("GITLAB_TOKEN") == "" {
		log.Fatal("missing gitlab token\n")
	}

	if os.Getenv("DEPENDENCY_LOGGING") == "DEBUG" {
		projects.Debug = true
	}
	
	s, err := projects.New("https://source.tui", os.Getenv("GITLAB_TOKEN"))
	if err != nil {
		log.Fatalf("failed to setup source: %s\n", err)
	}

	err = s.PlotDependencyTree(4186)
	if err != nil {
		log.Fatalf("failed to gather project data: %s", err)
	}
}
