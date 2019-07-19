package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/user"
	"path"
	"sort"
	"strings"

	"cw-sdk-go/client/rest"
	"cw-sdk-go/client/websocket"
	"cw-sdk-go/common"
	"github.com/juju/errors"
	"github.com/rivo/tview"
	"github.com/spf13/pflag"
)

var (
	marketStrings = pflag.StringSlice("market", []string{}, "Market to handle, like kraken/btc/usd. This flag can be given multiple times.")

	configFilename *string

	streamURL = pflag.String("streamurl", "", "Websocket Stream URL. By default, the production cryptowatch URL is used.")
	brokerURL = pflag.String("brokerurl", "", "Websocket Broker URL. By default, the production cryptowatch URL is used.")

	config *Config
	authn  *ConfigAuthn
)

func main() {
	configFilenameDefault := getConfigFilenameDefault()
	configFilename = pflag.String("config-file", configFilenameDefault, "Config filename to use")

	pflag.Parse()

	var err error

	config, err = LoadConfig(*configFilename)
	if err != nil {
		printConfigAuthnError(err)
		os.Exit(1)
	}

	authn, err = config.GetAuthn(*streamURL, *brokerURL)
	if err != nil {
		printConfigAuthnError(err)
		os.Exit(1)
	}

	if len(*marketStrings) == 0 {
		fmt.Fprintf(os.Stderr, "No markets are given, use --market <exchange>/<base>/<quote>\n")
		os.Exit(1)
	}

	statesTracker := NewConnStateTracker()

	markets, marketDescrByID := getMarketDescrs()

	app := tview.NewApplication()

	mainView := NewMainView(&MainViewParams{
		App:     app,
		Markets: markets,
	})

	sc, err := setupStreamClient(markets, mainView, statesTracker)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error setting up stream client: %s", err.Error())
		os.Exit(1)
	}

	tc, err := setupTradeClient(markets, marketDescrByID, mainView, statesTracker)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error setting up trade client: %s", err.Error())
		os.Exit(1)
	}

	fmt.Println("Starting UI ...")
	if err := app.SetRoot(mainView.GetUIPrimitive(), true).Run(); err != nil {
		panic(err)
	}

	// We end up here when the user quits the UI

	fmt.Println("")
	fmt.Println("Closing connections...")

	sc.Close()
	tc.Close()

	fmt.Println("Have a nice day.")
}

func getStateListener(
	statesTracker *ConnStateTracker,
	mainView *MainView,
	cc ConnComponent,
) websocket.StateCallback {
	return func(oldState, state websocket.ConnState) {
		summary := statesTracker.SetComponentState(cc, state)

		if summary.AllConnected {
			mainView.HideMessagebox("connectivity_issues")
		} else {
			mainView.ShowMessagebox(
				"connectivity_issues",
				"Connecting",
				getConnectivityStatusMessage(summary),
				&MessageboxParams{
					Buttons: []string{},
				},
			)
		}
	}
}

func getConnectivityStatusMessage(summary *ConnStateSummary) string {
	return fmt.Sprintf(
		"Stream: %s\n\nTrading: %s",
		getComponentConnectivityStatusMessage(summary.States[ConnComponentStream]),
		getComponentConnectivityStatusMessage(summary.States[ConnComponentTrading]),
	)
}

func getComponentConnectivityStatusMessage(s ConnComponentState) string {
	ret := websocket.ConnStateNames[s.State]
	if s.State != websocket.ConnStateEstablished && s.LastErr != nil {
		ret += fmt.Sprintf(" (%s)", s.LastErr.Error())
	}

	return ret
}

func getMarketDescrs() (markets []MarketDescr, marketDescrByID map[common.MarketID]MarketDescr) {
	rest := rest.NewCWRESTClient(nil)

	marketDescrByID = map[common.MarketID]MarketDescr{}

	mchan := make(chan MarketDescr)

	for _, ms := range *marketStrings {
		parts := strings.Split(ms, "/")
		if len(parts) != 3 {
			fmt.Fprintf(os.Stderr, "Invalid market %q, should contain 3 parts, like \"kraken/btc/usd\"\n", ms)
			os.Exit(1)
		}

		go func(exchange, base, quote string) {
			fmt.Printf("Getting descriptor of %s/%s/%s ...\n", exchange, base, quote)
			md, err := rest.GetMarketDescr(parts[0], parts[1]+parts[2])
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to get market %s/%s/%s: %s\n", exchange, base, quote, err)
				os.Exit(1)
			}

			fmt.Printf("Got %s/%s/%s\n", exchange, base, quote)

			mchan <- MarketDescr{
				ID:       common.MarketID(fmt.Sprintf("%d", md.ID)),
				Exchange: md.Exchange,
				Base:     strings.ToUpper(base),
				Quote:    strings.ToUpper(quote),
			}
		}(parts[0], parts[1], parts[2])
	}

	for i := 0; i < len(*marketStrings); i++ {
		market := <-mchan
		markets = append(markets, market)
		marketDescrByID[market.ID] = market
	}

	sort.Sort(MarketDescrsSorted(markets))

	return markets, marketDescrByID
}

func setupStreamClient(
	markets []MarketDescr,
	mainView *MainView,
	statesTracker *ConnStateTracker,
) (*websocket.StreamClient, error) {
	streamSubs := make([]*websocket.StreamSubscription, 0, len(markets))
	for _, m := range markets {
		streamSubs = append(streamSubs, &websocket.StreamSubscription{
			Resource: fmt.Sprintf("markets:%s:book:spread", m.ID),
		})
	}

	sc, err := websocket.NewStreamClient(&websocket.StreamClientParams{
		WSParams: &websocket.WSParams{
			URL:       *streamURL,
			APIKey:    authn.APIKey,
			SecretKey: authn.SecretKey,
		},

		Subscriptions: streamSubs,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	sc.OnMarketUpdate(
		func(market common.Market, data common.MarketUpdate) {
			switch {
			case data.OrderBookSpreadUpdate != nil:
				spread := data.OrderBookSpreadUpdate
				mainView.SetMarketSpread(
					market.ID,
					spread.Bid,
					spread.Ask,
				)
			}
		},
	)

	sc.OnError(func(err error, disconnecting bool) {
		// If the client is going to disconnect because of that error, just save
		// the error to show later on the disconnection message.
		if disconnecting {
			statesTracker.SetComponentError(ConnComponentStream, err)
			return
		}

		// Otherwise, show the error message right away.
		mainView.ShowMessagebox(
			"stream_client_error_"+err.Error(),
			fmt.Sprintf("Stream client error"),
			err.Error(),
			nil,
		)
	})

	sc.OnStateChange(
		websocket.ConnStateAny,
		getStateListener(statesTracker, mainView, ConnComponentStream),
	)

	sc.Connect()

	return sc, nil
}

func setupTradeClient(
	markets []MarketDescr,
	marketDescrByID map[common.MarketID]MarketDescr,
	mainView *MainView,
	statesTracker *ConnStateTracker,
) (*websocket.TradeClient, error) {
	tradeSubs := make([]*websocket.TradeSubscription, 0, len(markets))
	for _, m := range markets {
		tradeSubs = append(tradeSubs, &websocket.TradeSubscription{
			MarketID: common.MarketID(fmt.Sprintf("%s", m.ID)),
		})
	}

	tc, err := websocket.NewTradeClient(&websocket.TradeClientParams{
		WSParams: &websocket.WSParams{
			URL:       *brokerURL,
			APIKey:    authn.APIKey,
			SecretKey: authn.SecretKey,
		},

		Subscriptions: tradeSubs,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	// firstTradesUpdate is set to true initially and once we've disconnected, so
	// that the next trades update will overwrite the whole trades list.
	// Subsequent updates will just append to the list.
	firstTradesUpdate := true

	tc.OnOrdersUpdate(func(marketID common.MarketID, orders []common.PrivateOrder) {
		mainView.SetMarketOrders(marketID, orders)
	})

	// Balances grep flag: Ki49fK
	// tc.OnBalancesUpdate(func(marketID common.MarketID, balances common.Balances) {
	// 	mainView.SetMarketBalances(marketID, balances)
	// })

	tc.OnTradesUpdate(func(marketID common.MarketID, trades []common.PrivateTrade) {
		// If this is the first trades update after we've connected, reset the
		// whole trades list; otherwise append to it.
		add := true
		if firstTradesUpdate {
			firstTradesUpdate = false
			add = false
		}

		mainView.SetMarketTrades(marketID, trades, add)
	})

	tc.OnMarketReadyChange(func(marketID common.MarketID, ready bool) {
		mainView.SetMarketReady(marketID, ready)

		if !ready {
			firstTradesUpdate = true
		}
	})

	tc.OnError(func(marketID common.MarketID, err error, disconnecting bool) {
		// If the client is going to disconnect because of that error, just save
		// the error to show later on the disconnection message.
		if disconnecting {
			statesTracker.SetComponentError(ConnComponentTrading, err)
			return
		}

		// Otherwise, show the error message right away.
		mainView.ShowMessagebox(
			"trade_client_error_"+err.Error(),
			fmt.Sprintf("%s error", marketDescrByID[marketID]),
			err.Error(),
			nil,
		)

	})

	tc.OnStateChange(
		websocket.ConnStateAny,
		getStateListener(statesTracker, mainView, ConnComponentTrading),
	)

	// Link UI requests to place/cancel orders with the trade client {{{
	mainView.SetOnPlaceOrderRequestCallback(
		func(orderOpt common.PlaceOrderOpt) {
			// It's called from the UI eventloop, so we can mess with UI right here.
			msgv := NewMessageView(mainView, &MessageViewParams{
				MessageID: "placing_order",
				Message:   "Placing order...",
			})
			msgv.Show()

			go func() {
				_, orderErr := tc.PlaceOrder(orderOpt)

				// This isn't UI eventloop, so we need to get there.
				mainView.params.App.QueueUpdateDraw(func() {
					msgv.Hide()

					if orderErr != nil {
						mainView.ShowMessagebox(
							"order_error", "", "Order failed: "+orderErr.Error(), nil,
						)
					}
				})
			}()
		},
	)

	mainView.SetOnCancelOrderRequestCallback(
		func(orderOpts common.CancelOrderOpt) {
			// It's called from the UI eventloop, so we can mess with UI right here.
			msgv := NewMessageView(mainView, &MessageViewParams{
				MessageID: "canceling_order",
				Message:   "Canceling order...",
			})
			msgv.Show()

			go func() {
				orderErr := tc.CancelOrder(orderOpts)

				// This isn't UI eventloop, so we need to get there.
				mainView.params.App.QueueUpdateDraw(func() {
					msgv.Hide()

					if orderErr != nil {
						mainView.ShowMessagebox(
							"cancel_order_error", "", "Cancel order failed: "+orderErr.Error(), nil,
						)
					}
				})
			}()
		},
	)
	// }}}

	tc.Connect()

	return tc, nil
}

// getConfigFilenameDefault tries to get user home directory, and returns
// a path of config file in that directory. If failed to get home directory,
// then "/etc" is used instead.
func getConfigFilenameDefault() string {
	filename := ".cw_trading/config.json"
	etcPath := "/etc"

	usr, err := user.Current()
	if err != nil {
		return path.Join(etcPath, filename)
	}

	if usr.HomeDir == "" {
		return path.Join(etcPath, filename)
	}

	return path.Join(usr.HomeDir, filename)
}

// printConfigAuthnError prints passed authentication error as well as example
// copy-pasteable config JSON.
func printConfigAuthnError(err error) {
	fmt.Fprintf(os.Stderr, "Error getting authentication from config: %s\n", err.Error())

	exampleConfig := &Config{
		Authns: []ConfigAuthn{
			ConfigAuthn{
				StreamURL: *streamURL,
				BrokerURL: *brokerURL,

				APIKey:    "my_api_key",
				SecretKey: "my_secret_key",
			},
		},
	}

	fmt.Fprintf(os.Stderr, "Make sure your config file %s contains credentials, like this:\n", *configFilename)

	encoder := json.NewEncoder(os.Stderr)
	encoder.SetIndent("", "  ")
	encoder.Encode(exampleConfig)
}
