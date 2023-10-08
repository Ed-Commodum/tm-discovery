package main

import "flag"

const (
	defaultChainId     = "vega-mainnet-0011"
	defaultInitialRpcs = "165.232.126.207:26657,164.92.138.136:26657,185.246.86.71:26657,134.122.64.6:26657,39.59.237.19"
)

var (
	chainId      string
	initialRpcs  string
	inferChainId bool
	stateSync    bool
)

func init() {

	flag.StringVar(&chainId, "chain-id", defaultChainId, "The chain ID of the network for discovery.")
	flag.StringVar(&initialRpcs, "initial-rpcs", defaultInitialRpcs, "A comma separated list of initial TM RPC addresses")
	flag.BoolVar(&inferChainId, "infer-chain-id", false, "When flag is set, infer chain ID from initialRpcs")
	flag.BoolVar(&stateSync, "state-sync", false, "When true, generates TM configuration for state sync.")
}

func main() {

}
