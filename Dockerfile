FROM golang:1.21 AS builder
WORKDIR /opt/app

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s" -o /opt/app -v ./cmd/gitlab-ci-crawler/

FROM alpine:3.19
WORKDIR /opt/app
COPY --from=builder /opt/app/gitlab-ci-crawler .
USER 65534
CMD ["/opt/app/gitlab-ci-crawler"]
