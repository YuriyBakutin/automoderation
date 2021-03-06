package main

import (
	"encoding/json"
	"sync"
)

// Room maintains the set of active users and broadcasts messages to the users.
type Room struct {
	name string

	// Connected users.
	users map[*User]bool

	queues map[uint8]Queue
	mux    sync.Mutex

	send       chan []byte
	register   chan *User
	unregister chan *User
}

const minPriority = 0
const maxPriority = 4

func newRoom(name string) *Room {
	room := &Room{
		name:       name,
		send:       make(chan []byte),
		register:   make(chan *User),
		unregister: make(chan *User),
		users:      make(map[*User]bool),
		queues:     make(map[uint8]Queue),
	}

	var priority uint8
	for priority = minPriority; priority <= maxPriority; priority++ {
		room.queues[priority] = make(Queue, 0)
	}

	go room.run()
	return room
}

// Queue orders users for particular message priority
type Queue []*User

func (q *Queue) contains(user *User) bool {
	for _, u := range *q {
		if u == user {
			return true
		}
	}
	return false
}

func (room *Room) add(user *User, priority uint8) bool {
	queue, ok := room.queues[priority]
	if !ok || queue.contains(user) {
		return false
	}

	room.queues[priority] = append(queue, user)
	return true
}

func (room *Room) remove(user *User, priority uint8) bool {
	queue, ok := room.queues[priority]
	if !ok {
		return false
	}
	for index, u := range queue {
		if u == user {
			room.queues[priority] = append(queue[:index], queue[index+1:]...)
			return true
		}
	}
	return false
}

func (room *Room) sendCurrentState(user *User) {
	room.mux.Lock()
	defer room.mux.Unlock()

	for priority, queue := range room.queues {
		for _, author := range queue {
			user.broadcast(&BroadcatedMessage{
				Username: author.name,
				Priority: priority,
				Cancel:   false,
			})
		}
	}
}

func (room *Room) reserved(userName string) bool {
	room.mux.Lock()
	defer room.mux.Unlock()

	for user := range room.users {
		if user.name == userName {
			return true
		}
	}

	return false
}

// BroadcatedMessage represents the data broadcasted to users
type BroadcatedMessage struct {
	Username string `json:"user"`
	Priority uint8  `json:"priority"`
	Cancel   bool   `json:"cancel,omitempty"`
}

func (m *BroadcatedMessage) getBytes() []byte {
	bytes, _ := json.Marshal(m)
	return bytes
}

func (room *Room) broadcast(b *BroadcatedMessage) {
	bytesToSend := b.getBytes()

	for user, connected := range room.users {
		if !connected {
			continue
		}
		select {
		case user.send <- bytesToSend:
		default:
			room.unregister <- user
		}
	}
}

func (room *Room) run() {
	// TODO: die if no one is here
	for {
		select {
		case user := <-room.register:
			room.mux.Lock()
			room.users[user] = false
			room.mux.Unlock()

			room.sendCurrentState(user)
		case user := <-room.unregister:
			room.mux.Lock()
			prioritiesToClean := make([]uint8, 0)

			for priority := range room.queues {
				removed := room.remove(user, priority)
				if removed {
					prioritiesToClean = append(prioritiesToClean, priority)
				}
			}

			if _, ok := room.users[user]; ok {
				delete(room.users, user)
				user.conn.Close()
			}

			room.mux.Unlock()

			for _, priority := range prioritiesToClean {
				room.broadcast(&BroadcatedMessage{
					Username: user.name,
					Priority: priority,
					Cancel:   true,
				})
			}
		}
	}
}
