package main

import (
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net"

	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"

	rpcTypes "github.com/cometbft/cometbft/rpc/core/types"
	// rpcTypes "github.com/vegaprotocol/cometbft/rpc/jsonrpc/types"
)

type finder struct {
	mu sync.Mutex

	chainId     string
	initialRpcs string

	stateSync bool

	toCheck chan rpcTypes.Peer

	client http.Client

	numRpcsRunning   int
	numRpcsFinished  int
	numPeersRunning  int
	numPeersFinished int

	ips map[string]struct{}

	successfulRpcs []string
	failedRpcs     []string

	successfulPeers []string
	failedPeers     []string

	stopChan chan os.Signal
}

func NewFinder(config *Config) *finder {

	// Check whether chainId should be inferred from initial RPCs
	chainId := config.ChainId
	if config.InferChainId {
		chainId = ""
	}

	return &finder{
		mu:              sync.Mutex{},
		chainId:         chainId,
		initialRpcs:     config.InitialRpcs,
		stateSync:       config.StateSync,
		toCheck:         make(chan rpcTypes.Peer),
		client:          http.Client{Timeout: 5 * time.Second},
		numRpcsRunning:  0,
		numRpcsFinished: 0,
		ips:             make(map[string]struct{}),
		successfulRpcs:  []string{},
		failedRpcs:      []string{},
		successfulPeers: []string{},
		failedPeers:     []string{},
		stopChan:        make(chan os.Signal),
	}
}

// Begins the search using the set of inital RPCs
func (finder *finder) Start() {

	ticker := time.NewTicker(time.Second * 3)

	go func() {
		for {
			select {
			case <-ticker.C:
				finder.mu.Lock()
				fmt.Printf("Testing RPCs... %v running and %v found\n", finder.numRpcsRunning, len(finder.successfulRpcs))
				fmt.Printf("Testing Peers... %v running and %v found\nn", finder.numPeersRunning, len(finder.successfulPeers))
				if finder.numRpcsRunning == 0 && finder.numPeersRunning == 0 {
					fmt.Printf("All IPs tested.\n\n")
					fmt.Printf("Successful RPCs: \"%v\"\n\n", strings.Join(finder.successfulRpcs, ","))
					fmt.Printf("Successful Peers: \"%v\"\n\n", strings.Join(finder.successfulPeers, ","))
					if finder.stateSync {
						finder.generateStateSyncConfig()
					}
					finder.stopChan <- syscall.SIGTERM
				}
				finder.mu.Unlock()
			case peer := <-finder.toCheck:
				// Check RPC
				go finder.callRpc(fmt.Sprintf("http://%v:26657", peer.RemoteIP))

				// Check peer (better version will attempt to connect to peer
				go finder.dialPeer(peer)
			}
		}
	}()

	rpcs := strings.Split(finder.initialRpcs, ",")
	for _, rpcAddr := range rpcs {
		go finder.callRpc(rpcAddr)
	}
}

type jsonResponse struct {
	JsonRpc string         `json:"jsonrpc"`
	Id      int            `json:"id"`
	Result  map[string]any `json:"result"`
}

type jsonResponseNetInfo struct {
	JsonRpc string                 `json:"jsonrpc"`
	Id      int                    `json:"id"`
	Result  rpcTypes.ResultNetInfo `json:"result"`
}

func (finder *finder) callRpc(rpcAddr string) {

	finder.mu.Lock()
	finder.numRpcsRunning++
	finder.mu.Unlock()

	res, err := finder.client.Get(rpcAddr + "/net_info")
	if err != nil {
		// log.Printf("Failed to get response from %q: %v\n", rpcAddr, err)
		finder.mu.Lock()
		finder.failedRpcs = append(finder.failedRpcs, rpcAddr)
		finder.numRpcsRunning--
		finder.numRpcsFinished++
		finder.mu.Unlock()
		return
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		// log.Printf("Failed to read repsonse body: %v", err)
		finder.mu.Lock()
		finder.failedRpcs = append(finder.failedRpcs, rpcAddr)
		finder.numRpcsRunning--
		finder.numRpcsFinished++
		finder.mu.Unlock()
		return
	}

	decoded := &jsonResponseNetInfo{}
	json.Unmarshal(body, decoded)

	// fmt.Printf("Response body: %v\n", decoded)
	// fmt.Printf("Peers : %+v\n", decoded.Result.Peers)

	for _, peer := range decoded.Result.Peers {

		finder.mu.Lock()
		if _, ok := finder.ips[peer.RemoteIP]; ok {
			// Already checked/checking
			finder.mu.Unlock()
			continue
		}

		// Check chainId
		if finder.chainId == "" {
			finder.chainId = peer.NodeInfo.Network
		} else if finder.chainId != peer.NodeInfo.Network {
			log.Fatal("Multiple chainIds detected, ensure all initialRPCs are on the same network. \n Exiting.")
		}

		finder.ips[peer.RemoteIP] = struct{}{}
		finder.mu.Unlock()
		finder.toCheck <- peer
	}

	finder.mu.Lock()
	finder.numRpcsFinished++
	finder.numRpcsRunning--
	finder.successfulRpcs = append(finder.successfulRpcs, rpcAddr)
	finder.mu.Unlock()
}

func (finder *finder) dialPeer(peer rpcTypes.Peer) {

	finder.mu.Lock()
	finder.numPeersRunning++
	finder.mu.Unlock()

	peerAddr := string(peer.NodeInfo.DefaultNodeID) + "@" + peer.RemoteIP + ":26656"
	conn, err := net.DialTimeout("tcp", peer.RemoteIP+":26656", time.Second*3)
	if err != nil {
		// log.Printf("Couldn't connect to peer at %v", peerAddr)
		finder.mu.Lock()
		finder.numPeersRunning--
		finder.numPeersFinished++
		finder.failedPeers = append(finder.successfulPeers, peerAddr)
		finder.mu.Unlock()
		return
	}

	// fmt.Printf("Dialed peer at %v", peerAddr)
	finder.mu.Lock()
	finder.numPeersRunning--
	finder.numPeersFinished++
	finder.successfulPeers = append(finder.successfulPeers, peerAddr)
	finder.mu.Unlock()
	conn.Close()
}

func (finder *finder) generateStateSyncConfig() {

	idx := rand.Intn(len(finder.successfulRpcs) - 1)

	// Select two RPCs for statesync config
	rpcs := []string{finder.successfulRpcs[idx], finder.successfulRpcs[idx+1]}

	// Call RPC to get recent block height and block hash
	res, err := finder.client.Get(rpcs[0] + "/block")
	if err != nil {
		log.Fatalf("Failed to get recent block: %v\n", err)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		log.Fatalf("Failed to unmarshal json response: %v\n", err)
	}

	decoded := &jsonResponse{}
	json.Unmarshal(body, decoded)

	blockHash := decoded.Result["block_id"].(map[string]any)["hash"]
	blockHeight := decoded.Result["block"].(map[string]any)["header"].(map[string]any)["height"]

	// fmt.Printf("Block Hash: %v\n", blockHash)
	// fmt.Printf("Block Height: %v\n", blockHeight)

	stateSyncConfigStr := fmt.Sprintf("rpc_servers = \"%v\"\ntrust_height = %v\ntrust_hash = \"%v\"\n", strings.Join(rpcs, ","), blockHeight, blockHash)

	fmt.Printf("Statesync config:\n\n%v\n", stateSyncConfigStr)

	// rpc_servers = "sn010.validators-testnet.vega.xyz:40107,sn011.validators-testnet.vega.xyz:40117"
	// trust_height = 0
	// trust_hash = ""

}
