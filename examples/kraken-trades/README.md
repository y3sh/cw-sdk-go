# Kraken trades

This is an example application of stream-client-go library, it subscribes
for trades on all Kraken markets, and prints them. Example output:

```
$ go run kraken-trades.go -apikey=myapikey -secretkey=mysecretkey
2018/05/29 23:07:10 State updated: disconnected -> connecting
2018/05/29 23:07:11 State updated: connecting -> authenticating
2018/05/29 23:07:11 State updated: authenticating -> established
2018/05/29 23:07:12 Trade: bcheur: price: 853, amount: 0.7371
2018/05/29 23:07:15 Trade: ethbtc: price: 0.0756, amount: 0.39
2018/05/29 23:07:16 Trade: ethusd: price: 564.2, amount: 0.05
2018/05/29 23:07:16 Trade: ethusd: price: 564.2, amount: 0.05
2018/05/29 23:07:16 Trade: ethusd: price: 564.2, amount: 0.05
2018/05/29 23:07:16 Trade: ethusd: price: 564.19, amount: 0.3
2018/05/29 23:07:16 Trade: ethusd: price: 564.19, amount: 0.35
2018/05/29 23:07:16 Trade: ethusd: price: 564.2, amount: 0.05
2018/05/29 23:07:16 Trade: ethusd: price: 564.19, amount: 0.05
2018/05/29 23:07:16 Trade: ethusd: price: 564.19, amount: 0.00000335
2018/05/29 23:07:16 Trade: ethusd: price: 564.2, amount: 0.05
....
```
