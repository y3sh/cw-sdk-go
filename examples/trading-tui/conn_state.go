package main

import (
	"sync"

	"cw-sdk-go/client/websocket"
)

type ConnComponent int

const (
	ConnComponentStream ConnComponent = iota
	ConnComponentTrading

	NumConnComponents
)

type ConnComponentState struct {
	State   websocket.ConnState
	LastErr error
}

type ConnStateTracker struct {
	states map[ConnComponent]ConnComponentState
	mtx    sync.Mutex
}

func NewConnStateTracker() *ConnStateTracker {
	return &ConnStateTracker{
		states: map[ConnComponent]ConnComponentState{},
	}
}

type ConnStateSummary struct {
	AllConnected bool
	States       map[ConnComponent]ConnComponentState
}

// SetComponentState sets component state and returns whether all componets are
// connected.
func (cs *ConnStateTracker) SetComponentState(cc ConnComponent, state websocket.ConnState) *ConnStateSummary {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	s := cs.states[cc]
	s.State = state
	cs.states[cc] = s

	summary := cs.getSummaryInternal()
	return summary
}

func (cs *ConnStateTracker) SetComponentError(cc ConnComponent, err error) *ConnStateSummary {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	s := cs.states[cc]
	s.LastErr = err
	cs.states[cc] = s

	summary := cs.getSummaryInternal()
	return summary
}

func (cs *ConnStateTracker) getSummaryInternal() *ConnStateSummary {
	summary := &ConnStateSummary{
		AllConnected: true,
		States:       map[ConnComponent]ConnComponentState{},
	}
	for cc := ConnComponent(0); cc < NumConnComponents; cc++ {
		item := cs.states[cc]
		if item.State != websocket.ConnStateEstablished {
			summary.AllConnected = false
		}
		summary.States[cc] = item
	}

	return summary
}
