package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"

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

	// Get market descriptions, to know symbols (like "btcusd") by integer ID
	marketsSlice, err := restclient.GetMarketsIndex()
	if err != nil {
		log.Fatalf("failed to get markets of %s: %s", exchangeSymbol, err)
	}

	markets := map[int]rest.MarketDescr{}
	for _, market := range marketsSlice {
		markets[market.ID] = market
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
				Resource: "markets:*:trades",
			},
		},
	})
	if err != nil {
		log.Fatalf("%s", err)
	}

	var lastError error

	c.OnSubscriptionResult(func(sr websocket.SubscriptionResult) {
		fmt.Println(sr)
	})

	c.OnError(func(err error, disconnecting bool) {
		if disconnecting {
			lastError = err
			return
		}

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
				fmt.Printf(
					"%v %34s %16s %16s\n",
					trade.Timestamp, market.ExchangeID+" "+strings.ToUpper(market.CurrencyPairID), trade.Price, trade.Amount,
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
