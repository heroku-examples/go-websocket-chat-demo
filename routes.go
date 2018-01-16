package main

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/rmattam/go-websocket-chat-demo/chat"
)

// ChatSocket : struct to define the route parameters for the chat websocket.
type ChatSocket struct {
	Name        string
	Pattern     string
	Channel     string
	HandlerFunc func(*chat.Hub, http.ResponseWriter, *http.Request)
}

// ChatSockets : list of Route parameters for chat websocket configuration.
type ChatSockets []ChatSocket

// NewRouter : Initialize the router with the parameters provided.
func NewRouter(redisURL string) *mux.Router {
	r := mux.NewRouter().StrictSlash(true)

	for _, route := range chatsockets {
		route := route
		Hub := &chat.Hub{}
		Hub.SubsribeRedis(redisURL, route.Channel)
		r.
			Methods("GET").
			Path(route.Pattern).
			Name(route.Name).
			HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				route.HandlerFunc(Hub, w, r)
			})
	}

	//Setting up file servers
	r.PathPrefix("/").Handler(http.StripPrefix("/", http.FileServer(http.Dir("./public"))))
	return r
}

var chatsockets = ChatSockets{
	ChatSocket{
		Name:        "Chat1 Websocket Endpoint",
		Pattern:     "/ws/chat1",
		Channel:     "chat1",
		HandlerFunc: chat.HandleWebsocket,
	},
	ChatSocket{
		Name:        "Chat1 Websocket Endpoint",
		Pattern:     "/ws/chat2",
		Channel:     "chat2",
		HandlerFunc: chat.HandleWebsocket,
	},
}
