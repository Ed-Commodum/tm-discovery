package main

import (
	"flag"
	"fmt"
	// "log"
	"os"
	"strconv"
)

type Config struct {
	ChainId      string
	InitialRpcs  string
	InferChainId bool
	StateSync    bool
}

func parseFlags() *Config {
	flag.Parse()
	checkDefaults()
	if chainId = getFlag(chainId, os.Getenv("TMD_CHAIN_ID")); len(chainId) <= 0 {
		fmt.Printf("Using default chain-id: %v\n", defaultChainId)
		chainId = defaultChainId
	}
	if initialRpcs = getFlag(initialRpcs, os.Getenv("TMD_INITIAL_RPCS")); len(initialRpcs) <= 0 {
		fmt.Printf("Using default initial RPCs: %v\n", defaultInitialRpcs)
		initialRpcs = defaultInitialRpcs
	}
	if !inferChainId {
		inferChainId = getenvBool(os.Getenv("TMD_INFER_CHAIN_ID"))
	}
	if !stateSync {
		stateSync = getenvBool(os.Getenv("TMD_STATE_SYNC"))
	}
	return &Config{
		ChainId:      chainId,
		InitialRpcs:  initialRpcs,
		InferChainId: inferChainId,
		StateSync:    stateSync,
	}
}

func getFlag(flag, env string) string {
	if len(flag) <= 0 {
		return env
	}
	return flag
}

func getenvBool(env string) bool {
	val, _ := strconv.ParseBool(env)
	return val
}

func checkDefaults() {
	flags := map[string]*flag.Flag{}
	flag.VisitAll(func(f *flag.Flag) {
		if f.Name == "infer-chain-id" || f.Name == "state-sync" {
			return
		}
		flags[f.Name] = f
	})

	isSet := map[string]bool{"chain-id": false, "initial-rpcs": false}
	for name := range isSet {
		flag.Visit(func(f *flag.Flag) {
			if f.Name == name {
				isSet[name] = true
			}
		})
	}
	for name, flag := range flags {
		if !isSet[name] {
			fmt.Printf("%v flag not set, using default %v\n", flag.Name, flag.Value)
		}
	}
}
