# Builder stage
FROM golang:1.24.6 as builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /tix-go ./cmd/tixgo

# Final stage
FROM alpine:latest

WORKDIR /root/

COPY --from=builder /tix-go .

EXPOSE 8080

CMD ["./tix-go"]
