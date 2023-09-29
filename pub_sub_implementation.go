package session

import (
	"fmt"
	"sync"
)

// since go does not have a standard library option for pub/sub, only single listener channels, this is my implementation for a project of mine.

type ResponseBroadcaster struct {
	sync.Mutex
	listeners map[uint16]*ListenerNode
}

type ListenerNode struct {
	sync.Mutex
	ch     chan uint16
	closed bool
}

func NewResponseBroadcaster() *ResponseBroadcaster {
	return &ResponseBroadcaster{listeners: make(map[uint16]*ListenerNode)}
}

func (r *ResponseBroadcaster) AddListener(id uint16, ch chan uint16) error {
	r.Lock()
	if _, ok := r.listeners[id]; ok {
		return fmt.Errorf("packet id %d already has a listener", id)
	}

	r.listeners[id] = &ListenerNode{ch: ch, closed: false}
	r.Unlock()
	return nil
}

func (r *ResponseBroadcaster) GetListener(id uint16) (*ListenerNode, bool) {
	r.Lock()
	node, ok := r.listeners[id]
	r.Unlock()
	return node, ok
}

func (r *ResponseBroadcaster) RemoveAndCloseListener(id uint16, ch chan uint16) {
	r.Lock()
	node, ok := r.listeners[id]
	delete(r.listeners, id)
	r.Unlock()

	if !ok {
		close(ch)
	} else {
		node.Lock()
		node.closed = true
		close(ch)
		node.Unlock()
	}
}
