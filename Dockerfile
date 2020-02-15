FROM golang:1.12
COPY . /src
WORKDIR /src

RUN CGO_ENABLED=0 GOOS=linux go build -mod=vendor -o go-websocket-chat-demo .

FROM heroku/heroku:18
WORKDIR /app
COPY ./public /app/public
COPY --from=0 /src/go-websocket-chat-demo /app/
CMD ["/app/go-websocket-chat-demo"]