package orderbooks

import (
	"github.com/y3sh/cw-sdk-go/client/rest"
	"github.com/y3sh/cw-sdk-go/common"
)

// OrderBookSnapshotGetter gets the up-to-date snapshot. Typically clients
// should use OrderBookSnapshotGetterREST, which gets the snapshot from the
// REST API.
//
// This is needed in the first place because snapshots are broadcasted via
// websocket only every minute, so whenever the client just starts, or gets
// out of sync for a little bit, it needs to get the up-to-date snapshot
// to avoid waiting for it for too long from the websocket.
type OrderBookSnapshotGetter interface {
	GetOrderBookSnapshot() (common.OrderBookSnapshot, error)
}

var _ OrderBookSnapshotGetter = &OrderBookSnapshotGetterREST{}

// OrderBookSnapshotGetterREST implements OrderBookSnapshotGetter; it
// gets snapshot for the specified market from the REST API.
type OrderBookSnapshotGetterREST struct {
	client         *rest.CWRESTClient
	exchangeSymbol string
	pairSymbol     string
}

// NewOrderBookSnapshotGetterRESTBySymbol creates a new snapshot getter which
// uses the REST API to get snapshots for the given market.
func NewOrderBookSnapshotGetterRESTBySymbol(
	exchangeSymbol string, pairSymbol string, restParams *rest.CWRESTClientParams,
) *OrderBookSnapshotGetterREST {
	return &OrderBookSnapshotGetterREST{
		client:         rest.NewCWRESTClient(restParams),
		exchangeSymbol: exchangeSymbol,
		pairSymbol:     pairSymbol,
	}
}

func (sg *OrderBookSnapshotGetterREST) GetOrderBookSnapshot() (common.OrderBookSnapshot, error) {
	return sg.client.GetOrderBook(sg.exchangeSymbol, sg.pairSymbol)
}
