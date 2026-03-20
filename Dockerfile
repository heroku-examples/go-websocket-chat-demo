FROM golang:1.25 AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/go-websocket-chat-demo .

FROM heroku/heroku:24
WORKDIR /app
COPY --from=builder /app/go-websocket-chat-demo .
COPY public ./public
CMD ["./go-websocket-chat-demo"]
