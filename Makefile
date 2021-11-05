compile:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "-extldflags -static" -o ./bin/linux_x86/gitlab-ci-crawler ./cmd/gitlab-ci-crawler
