package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"github.com/y3sh/cw-sdk-go/client/websocket"
	"github.com/y3sh/cw-sdk-go/common"
)

var (
	subs stringSlice

	verbose = flag.Bool("verbose", false, "Prints all debug messages to stdout.")
	format  = flag.String("format", "json", "Data output format")

	credsFilename = flag.String("creds", "", "JSON file with credentials: the file must contain an object with two properties: \"api_key\" and \"secret_key\".")

	apiKey    = flag.String("apikey", "", "API key to use. Consider using -creds instead.")
	secretKey = flag.String("secretkey", "", "Secret key to use. Consider using -creds instead.")
)

func init() {
	flag.Var(&subs, "sub", "Subscription key. This flag can be given multiple times")
	flag.Parse()

	if *format != "json" {
		log.Fatalf("Invalid data format '%v'", *format)
	}
}

func main() {
	// Setup OS signal handler
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	args := flag.Args()

	// Get address to connect to
	u := ""
	if len(args) >= 1 {
		u = args[0]
	}

	// If --creds was given, use it; otherwise use --api-key and --secret-key.
	var cr *creds
	if *credsFilename != "" {
		var err error
		cr, err = parseCreds(*credsFilename)
		if err != nil {
			panic(err)
		}
	} else {
		cr = &creds{
			APIKey:    *apiKey,
			SecretKey: *secretKey,
		}
	}

	streamSubs := make([]*websocket.StreamSubscription, 0, len(subs))
	for _, v := range subs {
		streamSubs = append(streamSubs, &websocket.StreamSubscription{Resource: v})
	}

	// Setup market connection (but don't connect just yet)
	c, err := websocket.NewStreamClient(&websocket.StreamClientParams{
		WSParams: &websocket.WSParams{
			URL:       u,
			APIKey:    cr.APIKey,
			SecretKey: cr.SecretKey,
		},

		Subscriptions: streamSubs,
	})

	if err != nil {
		log.Fatalf("%s", err)
	}

	// Will print state changes to the user
	if *verbose {
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

		c.OnStateChange(
			websocket.ConnStateAny,
			func(oldState, state websocket.ConnState) {
				fmt.Printf("State updated: %s -> %s", websocket.ConnStateNames[oldState], websocket.ConnStateNames[state])
				if lastError != nil {
					fmt.Printf(" (%s)", lastError)
					lastError = nil
				}
				fmt.Printf("\n")
			},
		)
	}

	// Will print received market update messages
	c.OnMarketUpdate(func(market common.Market, update common.MarketUpdate) {
		fmt.Printf("Update on market %s: %+v\n", market.ID, update)
	})

	c.OnPairUpdate(func(pair common.Pair, update common.PairUpdate) {
		fmt.Printf("Pair update on pair %s: %+v\n", pair.ID, update)
	})

	c.OnSubscriptionResult(func(result websocket.SubscriptionResult) {
		fmt.Printf("Subscription result: %+v\n", result)
	})

	c.OnUnsubscriptionResult(func(result websocket.UnsubscriptionResult) {
		fmt.Printf("Unsubscription result: %+v\n", result)
	})

	// Start connection loop
	if *verbose {
		fmt.Printf("Connecting to %s ...\n", c.URL())
	}
	c.Connect()

	// Wait until the OS signal is received, at which point we'll close the
	// connection and quit
	<-interrupt
	fmt.Printf("Closing connection...\n")

	if err := c.Close(); err != nil {
		fmt.Printf("Failed to close connection: %s", err)
	}
}

// Output a protobuf message as a JSON string to stdout
func outputProtoJSON(msg interface{}) {
	m := &jsonpb.Marshaler{EmitDefaults: true}
	if pMsg, ok := msg.(proto.Message); ok {
		str, err := m.MarshalToString(pMsg)
		if err != nil {
			panic(err)
		}
		fmt.Println(str)
	} else {
		fmt.Println(msg)
		panic("Error: bad data received")
	}
}
