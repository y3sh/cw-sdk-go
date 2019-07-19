// Copyright 2018 Cryptowatch. All Rights Reserved.
// Use of this source code is governed by a BSD-style
// license which can be found in the LICENSE file.

/*
Package websocket provides a client for managing connections to the Cryptowatch Websocket API. The API consists of
two separate back ends for streaming and trading. Although they are separate services, both share
the same underlying connection logic. Each service has its own respective client: StreamClient and TradeClient.

Cryptowatch Websocket API

View the full websocket api documentation here: https://cryptowat.ch/docs/websocket-api

Connecting

To connect to either the streaming or trading back end, you will need an API key pair. Please inquire about
obtaining API keys here: https://docs.google.com/forms/d/e/1FAIpQLSdhv_ceVtKA0qQcW6zQzBniRBaZ_cC4al31lDCeZirntkmWQw/viewform?c=0&w=1

The same API key pair will work for both streaming and trading, although
we use specific access control rules on our back end to determine if your key pair can access any particular
data subscription or trading functionality.

WSParams

Both StreamClient and TradeClient use WSParams to specify connection options.

	type WSParams struct {
		// Required
		APIKey        string
		SecretKey     string

		// Not required
		URL           string
		ReconnectOpts *ReconnectOpts
	}

Typically you will only need to supply APIKey, SecretKey, and Subscriptions as the other parameters
have default values.

URL is the url of the back end to connect to. You will not need to supply it unless you are
testing against a non-production environment.

ReconnectOpts determine how (and if) the client should reconnect. By default, the client will
reconnect with linear backoff up to 30 seconds.

Subscriptions

The Subscriptions supplied in StreamClientParams determine what resources will be streamed.
Read the streaming subscriptions documentation here: https://cryptowat.ch/docs/websocket-api#data-streaming

The Subscriptions supplied in TradeClientParams determine what markets can be traded on. Read
about the trading subscription documentation here: https://cryptowat.ch/docs/websocket-api#trading

Both StreamClient and TradeClient can define OnSubscriptionResult which gives information about what was successfully subscribed to, and if there were any errors (e.g. Access Denied).

Basic Usage

The typical workflow is to create an instance of a client, set event handlers on it, then initiate the
connection. As events occur, the registered callbacks will be executed. For StreamClient, the callbacks
will contain data for the active subscriptions. For the TradeClient, callbacks will contain real-time
information about your open orders, trades, positions, or balances.

Stream Client

The stream client can be set up to process live market-level or pair-level data as follows:

	import "cw-sdk-go"

	client, err := websocket.NewStreamClient(&websocket.StreamClientParams{
		WSParams: &websocket.WSParams{
			APIKey:    "myapikey",
			SecretKey: "mysecretkey",
		},

		Subscriptions: []*websocket.StreamSubscription{
			&websocket.StreamSubscription{
				Resource: "markets:86:trades", // Trade feed for Kraken BTCEUR
			},
			&websocket.StreamSubscription{
				Resource: "markets:87:trades", // Trade feed for Kraken BTCEUR
			},
		},
	})
	if err != nil {
		log.Fatal("%s", err)
	}

	c.OnSubscriptionResult(func(sr websocket.SubscriptionResult) {
		// Verify subscriptions
	})

	client.OnError(func(err error, disconnecting bool) {
		// Handle errors
	})

	client.OnTradesUpdate(func(m websocket.Market, tu websocket.TradesUpdate) {
		// Handle live trade data
	})

	// Set more handlers for market and pair data

	client.Connect()

Trade Client

The trade client maintains the state of your orders, trades, balances, and positions, while also
allowing you to place and cancel orders. In order to start trading, you must wait for the
internal cache to initialize, which can be accomplished using the OnReady callback.

Each subscription represents a market you intend to trade and receive updates on. You can supply
API keys for the exchange, or the client will fall back on your API keys loaded in your Cryptowatch account.

	import (
		"cw-sdk-go"
		"cw-sdk-go/common"
	)

	client, err := websocket.NewTradeClient(&websocket.TradeClientParams{
		WSParams: &websocket.WSParams{
			APIKey:    "myapikey",
			SecretKey: "mysecretkey",
		},

		Subscriptions: []*websocket.TradeSubscription{
			&websocket.TradeSubscription{
				MarketID: common.MarketID("86"), // Trade on Kraken BTCEUR
			},
		},
	})
	if err != nil {
		log.Fatal("%s", err)
	}

	client.OnSubscriptionResult(func(sr websocket.SubscriptionResult) {
		// Verify subscriptions (trade sessions)
	})

	tc.OnError(func(mID common.MarketID, err error, disconnecting bool) {
		// Handle errors
	})

	client.OnReady(func() {
		// Start trading
	})

	// Set more handlers for orders, trades, positions, or balance updates

	client.Connect()

Error Handling and Connection States

Both StreamClient and TradeClient define an OnError callback which you can use
to respond to any error that may occur. The OnError callback function signature is
slightly different for trading because it includes a MarketID for any error that is
related to trading. The MarketID is blank for any error related to the connection itself.

The "disconnecting" argument is set to true if the error is going to cause the
disconnection: in this case, the app could store the error somewhere and show
it later, when the actual disconnection happens; see the example of that below,
together with state listener. Error handlers are always called before the state
change listeners.

Both StreamClient and TradeClient can set listeners for connection state
changes such as StateDisconnected, StateWaitBeforeReconnect, StateConnecting,
StateAuthenticating, and StateEstablished.  They can also listen for any state
changes by using StateAny. The following is an example of how you would print
verbose logs about a client's state transitions. "client" can represent either
StreamClient or TradeClient:

	var lastError error

	client.OnError(func(err error, disconnecting bool) {
		// If the client is going to disconnect because of that error, just save
		// the error to print later with the disconnection message.
		if disconnecting {
			lastError = err
			return
		}

		// Otherwise, print the error message right away.
		log.Printf("Error: %s", err.Error())
	})

	client.OnStateChange(
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

Strings Vs Floats

All price, ID, and subscription data is represented as strings. This is to prevent loss of
significant digits from using floats, as well as to provide a consistent and simple API.

Concurrency

All methods of the StreamClient and TradeClient can be called concurrently from any
number of goroutines. All callbacks and listeners are called by the same internal goroutine,
unique to each connection; that is, they are never called concurrently with each other.

Stream Client CLI

Use the command line tool stream-client to subscribe to live data feeds from the command line. To
install the tool, run "make" from the root of the repo. This will create the executable
bin/stream-client, which can be used as follows:

	./stream-client \
		-apikey=your_api_key \
		-secretkey=your_secret_key \
		-sub subscription_key \
		-sub subscription_key2

*/
package websocket
