package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
)

type finder struct {
	mu sync.Mutex

	chainId     string
	initialRpcs string

	toCheck <-chan string

	numRunning  int
	numFinished int

	successfulRpcs []string
	failedRpcs     []string

	successfulPeers []string
	failedPeers     []string
}

func NewFinder(config *Config) *finder {
	return &finder{
		mu:              sync.Mutex{},
		chainId:         config.ChainId,
		initialRpcs:     config.InitialRpcs,
		toCheck:         make(<-chan string),
		numRunning:      0,
		numFinished:     0,
		successfulRpcs:  []string{},
		failedRpcs:      []string{},
		successfulPeers: []string{},
		failedPeers:     []string{},
	}
}

// Begins the search using the set of inital RPCs
func (finder *finder) Start() {

	rpcs := strings.Split(finder.initialRpcs, ",")
	for _, rpcAddr := range rpcs {
		go finder.callRpc(rpcAddr)
	}
}

func (finder *finder) callRpc(rpcAddr string) {

	finder.numRunning++

	res, err := http.Get(rpcAddr)
	if err != nil {
		log.Printf("Failed to get response from %v: %v", rpcAddr, err)
		finder.mu.Lock()
		finder.failedRpcs = append(finder.failedRpcs, rpcAddr)
		finder.numRunning--
		finder.numFinished++
		finder.mu.Unlock()
		return
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		log.Printf("Failed to read repsonse body: %v", err)
		finder.mu.Lock()
		finder.failedRpcs = append(finder.failedRpcs, rpcAddr)
		finder.numRunning--
		finder.numFinished++
		finder.mu.Unlock()
		return
	}

	fmt.Printf("Response: %v", string(body))

}
