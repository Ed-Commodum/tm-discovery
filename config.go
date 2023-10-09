package main

import (
	"flag"
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
	if chainId = getFlag(chainId, os.Getenv("TMD_CHAIN_ID")); len(chainId) <= 0 {
		chainId = defaultChainId
	}
	if initialRpcs = getFlag(initialRpcs, os.Getenv("TMD_INITIAL_RPCS")); len(initialRpcs) <= 0 {
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
