ARG TAG="nonroot" 
FROM golang:1.14.1-buster AS builder

WORKDIR "/app"

COPY . .

# Create appuser
ENV GOOS=linux 
ENV GOARCH=amd64 
ENV GO111MODULE=on

RUN update-ca-certificates && \
    go mod download && \
    go mod verify && \
    CGO_ENABLED=0 go build -ldflags='-w -s -extldflags "-static"' -a -o /bin/s3-backup-restore cmd/s3-backup-restore/main.go
   
FROM gcr.io/distroless/static:${TAG}

COPY --from=builder /bin/s3-backup-restore /bin/s3-backup-restore

ENTRYPOINT ["/bin/s3-backup-restore"]

CMD ["cron"]
