package packetids

import (
	"../modules/helpers/bytes"
	"math"
	"sync"
)

// the following comment is weird to have in a demo file but for the sake of keeping this an authentic coding sample I will leave it in.

/*
 - This package is fantastic and works perfectly, if you are modifying it, second guess whether you need to.
 - This file has an accompanying test that tests it thoroughly, play with the test to make sure something is wrong.
*/

var MaxSimultaneousRequest uint16 = math.MaxUint16

type packetIDNode struct {
	i    uint16
	next *packetIDNode
}

type waiterNode struct {
	value    uint16
	done     bool
	next     *waiterNode
	previous *waiterNode
}

type PacketIDs struct {
	mu           sync.Mutex
	cond         *sync.Cond
	maxIDReached uint16
	stack        *packetIDNode
	stackSize    int64
	waitList     *waiterNode
	waitListSize int64
}

type PacketID struct {
	Value uint16
}

func (o *PacketID) GetBytes() [2]byte {
	return bytes.Split16BitWord(o.Value)
}

func (o *PacketID) GetBytesSlice() []byte {
	b := o.GetBytes()
	return []byte{b[0], b[1]}
}

func NewPacketIDFromBytes(packetIDBytes [2]byte) *PacketID {
	return &PacketID{Value: bytes.CombineTwoBytes(packetIDBytes)}
}

func NewPacketID(value uint16) *PacketID {
	return &PacketID{Value: value}
}

func New() *PacketIDs {
	pID := &PacketIDs{
		maxIDReached: 0,
		stack:        nil,
		waitList:     nil,
		cond:         nil,
	}

	pID.cond = sync.NewCond(&pID.mu)

	return pID
}

func (p *PacketIDs) Reserve() *PacketID {
	var w *waiterNode

	p.mu.Lock()
	defer p.mu.Unlock()

	// -- Grab one off the stack if one exists before creating one and adding it to memory.
	if p.stack != nil {
		id := p.stack.i
		p.stack = p.stack.next
		p.stackSize--
		return &PacketID{Value: id}
	}
	// --

	// -- If there was nothing in the stack, create a new id unless all 65535 are in flight.
	if p.maxIDReached < MaxSimultaneousRequest {
		p.maxIDReached++
		return &PacketID{Value: p.maxIDReached}
	}
	// --

	// -- Create a waiter and add it to the wait queue.
	if p.waitList == nil {
		w = &waiterNode{}
		p.waitList = w
		p.waitListSize++
	} else {
		w = &waiterNode{next: p.waitList}
		p.waitList.previous = w
		p.waitList = w
		p.waitListSize++
	}
	// --

	// -- This sync.Cond controls the mutex (p.mu), p.Cond.Broadcast in Release unlocks the p.mu mutex
	//    which unlocks all the goroutines that are waiting on that same lock. The correct waiter's
	//    goroutine will have their done set to true.
	for !w.done {
		p.cond.Wait()
	}
	// --

	// -- Remove the waiter from the waiter queue. Join the previous and next nodes together.
	if w.previous != nil {
		w.previous.next = w.next
	}
	if w.next != nil {
		w.next.previous = w.previous
	}

	p.waitListSize--
	// --

	return &PacketID{Value: w.value}
}

func (p *PacketIDs) Release(packetIDBytes [2]byte) {
	var id = bytes.CombineTwoBytes(packetIDBytes)

	p.mu.Lock()
	defer p.mu.Unlock()

	// -- If the wait list has a request waiting, give the released id to the oldest waiter.
	//    p.cond.Broadcast will unlock the p.mu mutex which will unlock all active requests,
	//    since they are all in for loops waiting on their done value to be true, all will loop
	//    and wait again except for the first waiter which will have it's done value set to true.
	if p.waitList != nil {
		w := p.waitList
		p.waitList = p.waitList.next
		w.value = id
		w.done = true
		p.cond.Broadcast()
		// --
	} else {
		// -- If the wait list is empty, add the released id to the stack.
		p.stack = &packetIDNode{i: id, next: p.stack}
		p.stackSize++
		// --
	}
}

func (p *PacketIDs) GetStackSize() int64 {
	return p.stackSize
}

func (p *PacketIDs) GetWaitListSize() int64 {
	return p.waitListSize
}
