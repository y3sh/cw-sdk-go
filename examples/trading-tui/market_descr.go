package main

import (
	"fmt"

	"github.com/y3sh/cw-sdk-go/common"
)

type MarketDescr struct {
	ID       common.MarketID
	Exchange string
	Base     string
	Quote    string
}

func (md MarketDescr) String() string {
	return fmt.Sprintf("%s %s%s", md.Exchange, md.Base, md.Quote)
}

// MarketDescrsSorted {{{
type MarketDescrsSorted []MarketDescr

func (mds MarketDescrsSorted) Len() int {
	return len(mds)
}

func (mds MarketDescrsSorted) Less(i, j int) bool {
	if mds[i].Exchange < mds[j].Exchange {
		return true
	} else if mds[i].Exchange > mds[j].Exchange {
		return false
	}

	if mds[i].Base < mds[j].Base {
		return true
	} else if mds[i].Base > mds[j].Base {
		return false
	}

	return mds[i].Quote < mds[j].Quote
}

func (mds MarketDescrsSorted) Swap(i, j int) {
	mds[i], mds[j] = mds[j], mds[i]
}

// }}}
