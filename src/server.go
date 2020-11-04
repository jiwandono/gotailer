package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"nhooyr.io/websocket"
	"sync"
	"time"
)

type gotailerServer struct {
	serveMux          http.ServeMux
	messageBufferSize int
	subscribers       map[*subscriber]struct{}
	subscribersMutex  sync.Mutex
}

type subscriber struct {
	messages  chan []byte
	closeSlow func()
}

func newGotailerServer() *gotailerServer {
	gs := gotailerServer{
		messageBufferSize: 256,
		subscribers:       make(map[*subscriber]struct{}),
	}

	gs.serveMux.Handle("/", http.FileServer(http.Dir("./public")))
	gs.serveMux.HandleFunc("/subscribe", gs.subscribeHandler)

	return &gs
}

func (server *gotailerServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	server.serveMux.ServeHTTP(w, r)
}

func (server *gotailerServer) subscribeHandler(w http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		fmt.Printf("subscribe error: %v\n", err)
		return
	}

	defer c.Close(websocket.StatusInternalError, "internal error")

	err = server.subscribe(r.Context(), c, r)
	if errors.Is(err, context.Canceled) {
		return
	}

	if websocket.CloseStatus(err) == websocket.StatusNormalClosure ||
		websocket.CloseStatus(err) == websocket.StatusGoingAway {
		return
	}

	if err != nil {
		fmt.Printf("subscribe error: %v\n", err)
		return
	}
}

func (server *gotailerServer) subscribe(ctx context.Context, c *websocket.Conn, r *http.Request) error {
	ctx = c.CloseRead(ctx)

	subscriber := &subscriber{
		messages: make(chan []byte, server.messageBufferSize),
		closeSlow: func() {
			_ = c.Close(websocket.StatusPolicyViolation, "connection too slow to keep up with messages")
		},
	}

	server.addSubscriber(subscriber)
	fmt.Printf("connected: %v, total subscribers: %d\n", r.RemoteAddr, len(server.subscribers))

	defer func() {
		server.removeSubscriber(subscriber)
		fmt.Printf("disconnected: %v, total subscribers: %d\n", r.RemoteAddr, len(server.subscribers))
	}()

	for {
		select {
		case message := <-subscriber.messages:
			err := writeTimeout(ctx, time.Second*5, c, message)
			if err != nil {
				return err
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (server *gotailerServer) publish(message []byte) {
	server.subscribersMutex.Lock()
	defer server.subscribersMutex.Unlock()

	for s := range server.subscribers {
		select {
		case s.messages <- message:
		default:
			go s.closeSlow()
		}
	}
}

func (server *gotailerServer) addSubscriber(subscriber *subscriber) {
	server.subscribersMutex.Lock()
	server.subscribers[subscriber] = struct{}{}
	server.subscribersMutex.Unlock()
}

func (server *gotailerServer) removeSubscriber(subscriber *subscriber) {
	server.subscribersMutex.Lock()
	delete(server.subscribers, subscriber)
	server.subscribersMutex.Unlock()
}

func writeTimeout(ctx context.Context, timeout time.Duration, c *websocket.Conn, msg []byte) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	return c.Write(ctx, websocket.MessageText, msg)
}
