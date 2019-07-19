package main

import (
	"encoding/json"
	"os"

	"github.com/juju/errors"
)

type creds struct {
	APIKey    string `json:"api_key"`
	SecretKey string `json:"secret_key"`
}

// parseCreds tries to parse a JSON file with creds; example file contents:
//
// {
//   "api_key": "foofoofoofoo",
//   "secret_key": "YmFyYmFyYmFyYmFy"
// }
func parseCreds(filename string) (*creds, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, errors.Annotatef(err, "opening creds file %q", filename)
	}
	defer f.Close()

	d := json.NewDecoder(f)

	ret := creds{}
	if err := d.Decode(&ret); err != nil {
		return nil, errors.Annotatef(err, "parsing JSON from %q", filename)
	}

	return &ret, nil
}
