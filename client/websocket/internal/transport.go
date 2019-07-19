package internal

import (
	"context"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/juju/errors"
)

type TransportState int

const (
	// TransportStateDisconnected means we're disconnected and not trying to connect.
	// connLoop is not running.
	TransportStateDisconnected TransportState = iota

	// TransportStateWaitBeforeReconnect means we already tried to connect, but then
	// either the connection failed, or succeeded but later disconnected for some
	// reason (see stateCause), and now we're waiting for a timeout before
	// connecting again. wsConn is nil, but connCtx and connCtxCancel are not,
	// and connLoop is running.
	TransportStateWaitBeforeReconnect

	// TransportStateConnecting means we're calling websocket.DefaultDialer.Dial() right
	// now.
	TransportStateConnecting

	// TransportStateConnected means the websocket connection is established.
	TransportStateConnected

	backoffIncrement = 500 * time.Millisecond

	// heartbeatPeriod is how often the server is expected to send heartbeats
	heartbeatPeriod = 10 * time.Second

	// readTimeout specifies how long to wait for any data received from the
	// server before disconnecting.
	readTimeout = heartbeatPeriod * 3
)

var (
	ErrNotConnected   = errors.New("transport error: not connected")
	ErrConnLoopActive = errors.New("transport error: connection loop is already active")
)

// StreamTransportParams contains params for opening a client stream connection
// (see StreamTransportConn)
type StreamTransportParams struct {
	URL string

	Reconnect           bool
	Backoff             bool
	ReconnectTimeout    time.Duration
	MaxReconnectTimeout time.Duration
}

// StreamTransportConn is a client stream connection; it's typically wrapped into more
// specific type of connection: e.g. MarketConn, which knows how to unmarshal
// the data being received.
type StreamTransportConn struct {
	params StreamTransportParams

	connTx chan WebsocketTx

	// Current state
	state TransportState
	// Error caused the current state; only relevant for TransportStateDisconnected and
	// TransportStateWaitBeforeReconnect, for other states it's always nil.
	stateCause error

	// onReadCB, if not nil, is called for each received websocket message.
	onReadCB onReadCallback

	// onStateChangeCB, if not nil, is called for each updated state.
	onStateChangeCB onStateChangeCallback

	// connCtx and connCtxCancel are context and its cancel func for the
	// currently running connLoop. If no connLoop is running at the moment (i.e.
	// the state is TransportStateDisconnected), these are nil.
	connCtx       context.Context
	connCtxCancel context.CancelFunc

	// wsConn is the currently active websocket connection, or nil if no
	// connection is established.
	wsConn *websocket.Conn

	// reconnectNow is a channel which is only non-nil in the
	// TransportStateWaitBeforeReconnect state, and closing it causes the reconnection to
	// happen immediately
	reconnectNow chan struct{}

	backoff              bool
	reconnectTimeout     time.Duration
	maxReconnectTimeout  time.Duration
	nextReconnectTimeout time.Duration

	mtx sync.Mutex
}

// websocketTx represents message to send to the websocket
type WebsocketTx struct {
	MessageType int
	Data        []byte
	Res         chan error
}

// NewStreamTransportConn creates a new stream transport connection.
//
// Note that a client should manually call Connect on a newly created
// connection; the rationale is that clients might register state and/or
// message handler before the connection, to avoid any possible races.
func NewStreamTransportConn(params *StreamTransportParams) (*StreamTransportConn, error) {
	c := &StreamTransportConn{
		// Copy params defensively
		params: *params,

		state:  TransportStateDisconnected,
		connTx: make(chan WebsocketTx, 1),
	}

	if c.params.Reconnect {
		// Set minimum ReconnectTimeout to 1 second if Backoff=false
		if !c.params.Backoff && c.params.ReconnectTimeout < 1*time.Second {
			c.params.ReconnectTimeout = 1 * time.Second
		}
		c.backoff = c.params.Backoff
		c.reconnectTimeout = c.params.ReconnectTimeout
		c.maxReconnectTimeout = c.params.MaxReconnectTimeout
	}

	// Start writeLoop right away, before even connecting, so that an attempt to
	// write something while not connected will result in a proper error.
	go c.writeLoop()

	return c, nil
}

// Connect either starts a connection goroutine (if state is
// TransportStateDisconnected), or makes it to stop waiting a timeout and connect right
// now (if state is TransportStateWaitBeforeReconnect). For other states, returns an
// error.
//
// It doesn't wait for the connection to establish, and returns immediately.
func (c *StreamTransportConn) Connect() error {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	switch c.state {
	case TransportStateDisconnected:
		// NOTE that we need to enter the state TransportStateConnecting here and not in
		// connLoop, in order to prevent the race which would result in multiple
		// running connLoops.
		c.updateState(TransportStateConnecting, nil)

		go c.connLoop(c.connCtx, c.connCtxCancel)

	case TransportStateWaitBeforeReconnect:
		// We're waiting for a timeout before reconnecting; force it to reconnect
		// right now
		close(c.reconnectNow)

	case TransportStateConnecting, TransportStateConnected:
		// Already connected or connecting
		return errors.Trace(ErrConnLoopActive)
	}

	return nil
}

// Close stops reconnection loop (if reconnection was requested), and if
// websocket connection is active at the moment, closes it as well (with the
// code 1000, i.e. normal closure). If graceful websocket closure fails, the
// forceful one is performed.
func (c *StreamTransportConn) Close() error {
	if err := c.CloseOpt(websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), true); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (c *StreamTransportConn) CloseOpt(data []byte, stopReconnecting bool) error {
	c.mtx.Lock()
	wsConn := c.wsConn

	if c.state == TransportStateDisconnected {
		c.mtx.Unlock()
		return errors.Trace(ErrNotConnected)
	}

	// If asked to stop reconnection, cancel the conn context, which will
	// cause connLoop to quit once the current websocket connection (if any)
	// is closed
	if stopReconnecting {
		c.connCtxCancel()
	}
	c.mtx.Unlock()

	// If websocket connection is active, close it, which will cause connLoop
	// break out of readLoop (and then either reconnect or quit, depending on the
	// stopReconnecting arg)
	if wsConn != nil {
		if err := wsConn.WriteControl(websocket.CloseMessage, data, time.Time{}); err != nil {
			// Graceful close failed, try to close forcefully
			return errors.Trace(wsConn.Close())
		}
	}

	return nil
}

// URL returns an url used for connection
func (c *StreamTransportConn) URL() string {
	return c.params.URL
}

// GetState returns connection state
func (c *StreamTransportConn) GetState() TransportState {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	return c.state
}

type onReadCallback func(conn *StreamTransportConn, data []byte)
type onStateChangeCallback func(conn *StreamTransportConn, oldState, state TransportState, cause error)

// OnRead sets on-read callback; it should be called once right after creation
// of the StreamTransportConn, before the connection is established.
func (c *StreamTransportConn) OnRead(cb onReadCallback) {
	c.onReadCB = cb
}

func (c *StreamTransportConn) OnStateChange(cb onStateChangeCallback) {
	c.onStateChangeCB = cb
}

// Send sends data to the websocket if it's connected
func (c *StreamTransportConn) Send(ctx context.Context, data []byte) error {
	// Note that we don't check here whether the socket is connected,
	// as it's checked by the writeLoop() which will receive our message
	// from c.connTx.

	res := make(chan error)

	// Request the websocket write
	c.connTx <- WebsocketTx{
		MessageType: websocket.TextMessage,
		Data:        data,
		Res:         res,
	}

	select {
	case err := <-res:
		if err != nil {
			return errors.Annotatef(err, "sending msg")
		}
	case <-ctx.Done():
		return errors.Trace(ctx.Err())
	}

	return nil
}

// enterLeaveState should be called on leaving and entering each state. So,
// when changing state from A to B, it's called twice, like this:
//
//      enterLeaveState(A, false)
//      enterLeaveState(B, true)
func (c *StreamTransportConn) enterLeaveState(state TransportState, enter bool) {
	switch state {

	case TransportStateDisconnected:
		// connCtx and its cancel func should be present in all states but
		// TransportStateDisconnected
		if enter {
			c.connCtx = nil
			c.connCtxCancel = nil
		} else {
			c.connCtx, c.connCtxCancel = context.WithCancel(context.Background())
		}

	case TransportStateWaitBeforeReconnect:
		// reconnectNow is present only in TransportStateWaitBeforeReconnect
		if enter {
			c.reconnectNow = make(chan struct{})
		} else {
			c.reconnectNow = nil
		}

	case TransportStateConnecting:
		// Nothing special to do for the TransportStateConnecting state

	case TransportStateConnected:
		// wsConn is present only in TransportStateConnected
		if enter {
			// wsConn is set by the calling code
		} else {
			c.wsConn = nil
		}
	}
}

func (c *StreamTransportConn) updateState(state TransportState, cause error) {
	// NOTE: c.mtx should be locked when updateState is called

	if c.state == state {
		// No need to do anything
		return
	}

	// Properly leave the current state
	c.enterLeaveState(c.state, false)

	oldState := c.state
	c.state = state
	c.stateCause = cause

	// Properly enter the new state
	c.enterLeaveState(c.state, true)

	if c.onStateChangeCB != nil {
		c.onStateChangeCB(c, oldState, state, cause)
	}
}

// connLoop establishes a connection, then keeps receiving all websocket
// messages (and calls onReadCB for each of them) until the connection is
// closed, then either waits for a timeout and connects again, or just quits.
func (c *StreamTransportConn) connLoop(connCtx context.Context, connCtxCancel context.CancelFunc) {
	var connErr error

	c.ResetTimeout()

	defer func() {
		c.mtx.Lock()
		defer c.mtx.Unlock()
		c.updateState(TransportStateDisconnected, connErr)
	}()

cloop:
	for {
		// When the goroutine is just started by Connect(), the state is already
		// TransportStateConnecting (see Connect() for the explanation on why), in which
		// case the updateState below is a no-op. When reconnecting though, the
		// state is different here, so it'll be changed to TransportStateConnecting.
		c.mtx.Lock()
		c.updateState(TransportStateConnecting, nil)
		c.mtx.Unlock()

		var wsConn *websocket.Conn
		wsConn, _, connErr = websocket.DefaultDialer.Dial(c.params.URL, nil)
		if connErr == nil {
			c.mtx.Lock()
			c.wsConn = wsConn
			c.updateState(TransportStateConnected, nil)
			c.mtx.Unlock()

			readTimer := time.AfterFunc(readTimeout, func() {
				// We haven't heard anything from the server for too long, so,
				// disconnect.
				//
				// NOTE that we don't use the c.Close() method: it's quite likely that
				// there is some problem with the network, and c.Close() first tries to
				// send the websocket close message, which would take quite some time
				// before timing out. Instead we just close the ws connection
				// forcefully, thus immediately breaking out of recvLoop,
				// so the clients will notice the state change right away.
				wsConn.Close()
			})

			// Will loop here until the websocket connection is closed
		recvLoop:
			for {
				msgType, data, err := wsConn.ReadMessage()
				if err != nil {
					connErr = err
					break recvLoop
				}

				// Just received something from the server: reset the read timeout. We
				// don't bother to check whether the timer has already fired: if so,
				// we'll reconnect anyway.
				readTimer.Reset(readTimeout)

				switch msgType {
				case websocket.TextMessage, websocket.BinaryMessage:
					if len(data) == 1 && data[0] == 0x01 {
						// Heartbeat, ignore
						continue recvLoop
					}

					// Call on-read callback, if any
					if c.onReadCB != nil {
						c.onReadCB(c, data)
					}

				case websocket.CloseMessage:
					break recvLoop
				}
			}

			// Cancel the read timeout. We don't bother to check whether the timer
			// has already fired: if so, we'll redundantly close the connection.
			readTimer.Stop()
		}

		// If shouldn't reconnect, we're done
		if !c.params.Reconnect {
			connCtxCancel()
		}

		// Check if we need to enter state TransportStateWaitBeforeReconnect
		select {
		case <-connCtx.Done():
			// Even though we have the same case in the select below, we want to
			// break cloop here, because if reconnection timeout is _also_ done, we
			// still want to break cloop instead of trying to reconnect.
			break cloop
		default:
			// Looks like we should reconnect (after a timeout), so set the
			// appropriate state
			c.mtx.Lock()
			c.updateState(TransportStateWaitBeforeReconnect, connErr)
			c.mtx.Unlock()
		}

		// Either wait for the timeout before reconnection, or quit.
	waitReconnect:
		select {
		case <-connCtx.Done():
			// Enough reconnections, quit now.
			break cloop

		case <-time.After(c.nextReconnectTimeout):
			// Will try to reconnect one more time
			break waitReconnect

		case <-c.reconnectNow:
			// Will try to reconnect one more time
			break waitReconnect
		}

		if c.backoff {
			c.nextReconnectTimeout += backoffIncrement
			if c.nextReconnectTimeout > c.maxReconnectTimeout {
				c.nextReconnectTimeout = c.maxReconnectTimeout
			}
		}
	}
}

// writeLoop receives messages from c.connTx, and tries to send them
// to the active websocket connection, if any.
func (c *StreamTransportConn) writeLoop() {
cloop:
	for {
		msg := <-c.connTx

		// Get currently active websocket connection
		c.mtx.Lock()
		wsConn := c.wsConn
		c.mtx.Unlock()

		if wsConn == nil {
			msg.Res <- errors.Trace(ErrNotConnected)
			continue cloop
		}

		// Try to write the message
		err := errors.Trace(wsConn.WriteMessage(msg.MessageType, msg.Data))

		// Send resulting error to the requester
		msg.Res <- err
	}
}

func (c *StreamTransportConn) ResetTimeout() {
	c.nextReconnectTimeout = c.reconnectTimeout
}
