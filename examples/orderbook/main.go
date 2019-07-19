package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"

	"github.com/y3sh/cw-sdk-go/client/rest"
	"github.com/y3sh/cw-sdk-go/client/websocket"
	"github.com/y3sh/cw-sdk-go/common"
	"github.com/y3sh/cw-sdk-go/orderbooks"
)

var (
	apiKey    = flag.String("apikey", "", "API key to use.")
	secretKey = flag.String("secretkey", "", "Secret key to use.")

	exchange = flag.String("exchange", "kraken", "")
	pair     = flag.String("pair", "btceur", "")

	streamURL = flag.String("streamurl", "", "Stream URL to use. By default, production URL is used.")
	apiURL    = flag.String("apiurl", "", "REST API URL to use. By default, production URL is used.")
)

func init() {
	flag.Parse()
}

// We'll use this instead of fmt.Println to add timestamps to our printed messages
var outlog = log.New(os.Stdout, "", log.LstdFlags)

func main() {
	restclient := rest.NewCWRESTClient(nil)

	// Get market description, in particular we'll need the ID to use it
	// in the stream subscription.
	market, err := restclient.GetMarketDescr(*exchange, *pair)
	if err != nil {
		log.Fatalf("failed to get market %s/%s: %s", *exchange, *pair, err)
	}

	// Create a new stream connection instance. Note that this does not yet
	// initiate the actual connection.
	c, err := websocket.NewStreamClient(&websocket.StreamClientParams{
		WSParams: &websocket.WSParams{
			URL:       *streamURL,
			APIKey:    *apiKey,
			SecretKey: *secretKey,
		},

		Subscriptions: []*websocket.StreamSubscription{
			&websocket.StreamSubscription{
				Resource: fmt.Sprintf("markets:%d:book:deltas", market.ID),
			},
			&websocket.StreamSubscription{
				Resource: fmt.Sprintf("markets:%d:book:snapshots", market.ID),
			},
		},
	})
	if err != nil {
		log.Fatalf("%s", err)
	}

	// Create orderbook updater: it handles all snapshots and deltas correctly,
	// keeping track of sequence numbers, fetching snapshots from the REST API
	// when appropriate and replaying cached deltas on them, etc. We'll only
	// need to feed it with snapshots and deltas we receive from the wire, and
	// subscribe on orderbook updates using OnOrderBookUpdate.
	orderbookUpdater := orderbooks.NewOrderBookUpdater(&orderbooks.OrderBookUpdaterParams{
		SnapshotGetter: orderbooks.NewOrderBookSnapshotGetterRESTBySymbol(
			*exchange, *pair, &rest.CWRESTClientParams{
				APIURL: *apiURL,
			},
		),
	})

	// Ask for the state transition updates and print them
	c.OnStateChange(
		websocket.ConnStateAny,
		func(oldState, state websocket.ConnState) {
			log.Printf(
				"State updated: %s -> %s",
				websocket.ConnStateNames[oldState],
				websocket.ConnStateNames[state],
			)
		},
	)

	// Listen for market changes
	c.OnMarketUpdate(
		func(market common.Market, md common.MarketUpdate) {
			if snapshot := md.OrderBookSnapshot; snapshot != nil {
				orderbookUpdater.ReceiveSnapshot(*snapshot)
			} else if delta := md.OrderBookDelta; delta != nil {
				// First, pretty-print the delta object.
				outlog.Println(PrettyDeltaUpdate{*delta}.String())

				orderbookUpdater.ReceiveDelta(*delta)
			}
		},
	)

	orderbookUpdater.OnUpdate(func(update orderbooks.Update) {
		if ob := update.OrderBookUpdate; ob != nil {
			log.Println("Updated orderbook:")
			orderbook := NewOrderBookMirror(*ob)
			outlog.Println(orderbook.PrettyString())
		} else if state := update.StateUpdate; state != nil {
			log.Printf("Updated state: %+v\n", state)
		} else if err := update.GetSnapshotError; err != nil {
			outlog.Println("Error", err)
		}
	})

	// Finally, connect.
	c.Connect()

	// Setup OS signal handler
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	// Wait until the OS signal is received, at which point we'll close the
	// connection and quit
	<-interrupt
	log.Printf("Closing connection...\n")

	if err := c.Close(); err != nil {
		log.Fatalf("Failed to close connection: %s", err)
	}
}
