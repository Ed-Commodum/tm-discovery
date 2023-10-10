package main

import (
	"flag"
	"fmt"
	"os/signal"
	"syscall"
)

const (
	defaultChainId     = "vega-mainnet-0011"
	defaultInitialRpcs = "http://165.232.126.207:26657,http://164.92.138.136:26657,http://185.246.86.71:26657,http://134.122.64.6:26657,http://39.59.237.19:26657"
)

var (
	chainId      string
	initialRpcs  string
	inferChainId bool
	stateSync    bool
)

func init() {
	fmt.Println("Preparing config...")
	flag.StringVar(&chainId, "chain-id", defaultChainId, "The chain ID of the network for discovery.")
	flag.StringVar(&initialRpcs, "initial-rpcs", defaultInitialRpcs, "A comma separated list of initial TM RPC addresses")
	flag.BoolVar(&inferChainId, "infer-chain-id", false, "When flag is set, infer chain ID from initialRpcs")
	flag.BoolVar(&stateSync, "state-sync", false, "When true, generates TM configuration for state sync.")
}

func main() {
	config := parseFlags()
	finder := NewFinder(config)
	finder.Start()

	signal.Notify(finder.stopChan, syscall.SIGTERM, syscall.SIGINT)
	<-finder.stopChan
}
