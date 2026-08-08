FROM golang:1.25-alpine AS builder
RUN apk add --no-cache git ca-certificates tzdata
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o bin/server ./cmd/server
RUN CGO_ENABLED=0 GOOS=linux go build -o bin/worker ./cmd/worker

FROM alpine:3.19
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app
COPY --from=builder /app/bin/server /app/server
COPY --from=builder /app/bin/worker /app/worker

EXPOSE 8080
CMD ["/app/server"]
