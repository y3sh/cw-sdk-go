package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"

	"y3sh-cw-sdk-go/client/rest"
	"y3sh-cw-sdk-go/client/websocket"
	"y3sh-cw-sdk-go/common"
)

const (
	exchangeSymbol = "kraken"
)

var (
	apiKey    = flag.String("apikey", "", "API key to use.")
	secretKey = flag.String("secretkey", "", "Secret key to use.")
)

func init() {
	flag.Parse()
}

func main() {
	restclient := rest.NewCWRESTClient(nil)

	// Get exchange description, in particular we'll need the ID to use it
	// in stream subscriptions
	exchange, err := restclient.GetExchangeDescr(exchangeSymbol)
	if err != nil {
		log.Fatalf("failed to get details of %s: %s", exchangeSymbol, err)
	}

	// Get market descriptions, to know symbols (like "btcusd") by integer ID
	marketsSlice, err := restclient.GetExchangeMarketsDescr(exchangeSymbol)
	if err != nil {
		log.Fatalf("failed to get markets of %s: %s", exchangeSymbol, err)
	}

	markets := map[common.MarketID]rest.MarketDescr{}
	for _, market := range marketsSlice {
		markets[common.MarketID(strconv.Itoa(market.ID))] = market
	}

	// Create a new stream connection instance. Note that the actual connection
	// will happen later.
	c, err := websocket.NewStreamClient(&websocket.StreamClientParams{
		WSParams: &websocket.WSParams{
			URL:       "wss://stream.cryptowat.ch",
			APIKey:    *apiKey,
			SecretKey: *secretKey,
		},

		Subscriptions: []*websocket.StreamSubscription{
			&websocket.StreamSubscription{
				Resource: fmt.Sprintf("exchanges:%d:trades", exchange.ID),
			},
		},
	})
	if err != nil {
		log.Fatalf("%s", err)
	}

	var lastError error

	c.OnError(func(err error, disconnecting bool) {
		// If the client is going to disconnect because of that error, just save
		// the error to show later on the disconnection message.
		if disconnecting {
			lastError = err
			return
		}

		// Otherwise, print the error message right away.
		log.Printf("Error: %s", err.Error())
	})

	// Ask for the state transition updates, and present them to the user somehow
	c.OnStateChange(
		websocket.ConnStateAny,
		func(oldState, state websocket.ConnState) {
			causeStr := ""
			if lastError != nil {
				causeStr = fmt.Sprintf(" (%s)", lastError)
				lastError = nil
			}
			log.Printf(
				"State updated: %s -> %s%s",
				websocket.ConnStateNames[oldState],
				websocket.ConnStateNames[state],
				causeStr,
			)
		},
	)

	// Listen for received market messages and print them
	c.OnMarketUpdate(func(market common.Market, md common.MarketUpdate) {
		if md.TradesUpdate != nil {
			tradesUpdate := md.TradesUpdate
			for _, trade := range tradesUpdate.Trades {
				log.Printf(
					"Trade: %s: price: %s, amount: %s",
					markets[market.ID].Pair, trade.Price, trade.Amount,
				)
			}
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
