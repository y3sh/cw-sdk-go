package main

import (
	"encoding/json"
	"io/ioutil"

	"github.com/juju/errors"
)

var (
	ErrNoAuthn = errors.New("no authentication found in config file")
)

type Config struct {
	// Authns is a slice of configured credentials for the client. When looking
	// for the creds to use, client iterates them one by one, and compares
	// requested streamURL and brokerURL with the values in ConfigAuthn. If
	// they match, these credentials are used.
	Authns []ConfigAuthn `json:"authns"`
}

type ConfigAuthn struct {
	StreamURL string `json:"streamUrl,omitempty"`
	BrokerURL string `json:"brokerUrl,omitempty"`

	APIKey    string `json:"apiKey"`
	SecretKey string `json:"secretKey"`
}

func LoadConfig(filename string) (*Config, error) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var cfg Config

	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, errors.Trace(err)
	}

	return &cfg, nil
}

// GetAuthn returns credentials for the given streamURL and brokerURL, or
// ErrNoAuthn of the config doesn't contain them.
func (c *Config) GetAuthn(streamURL, brokerURL string) (*ConfigAuthn, error) {
	for _, a := range c.Authns {
		if a.StreamURL == streamURL && a.BrokerURL == brokerURL {
			return &a, nil
		}
	}

	return nil, errors.Trace(ErrNoAuthn)
}
