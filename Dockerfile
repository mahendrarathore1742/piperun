# ---- Build ----
FROM golang:1.21-alpine AS builder
RUN apk add --no-cache git
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /usr/local/bin/piperun ./cmd/piperun

# ---- Runtime ----
FROM alpine:3.19
RUN apk add --no-cache bash git docker-cli
COPY --from=builder /usr/local/bin/piperun /usr/local/bin/piperun
ENTRYPOINT ["piperun"]
