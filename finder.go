package main

import (
	"context"
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

	vegaApiPb "code.vegaprotocol.io/vega/protos/vega/api/v1"
	rpcTypes "github.com/cometbft/cometbft/rpc/core/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	// rpcTypes "github.com/vegaprotocol/cometbft/rpc/jsonrpc/types"
)

type finder struct {
	mu sync.Mutex

	chainId     string
	initialRpcs string

	stateSync bool

	toCheck chan rpcTypes.Peer

	client http.Client

	numRpcsRunning      int
	numRpcsFinished     int
	numPeersRunning     int
	numPeersFinished    int
	numCoreApisRunning  int
	numCoreApisFinished int

	ips map[string]struct{}

	successfulRpcs []string
	failedRpcs     []string

	successfulPeers []string
	failedPeers     []string

	successfulCoreApis []string
	failedCoreApis     []string

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
				fmt.Printf("Testing Peers... %v running and %v found\n", finder.numPeersRunning, len(finder.successfulPeers))
				fmt.Printf("Testing Core APIs... %v running and %v found\n", finder.numCoreApisRunning, len(finder.successfulCoreApis))
				if finder.numRpcsRunning == 0 && finder.numPeersRunning == 0 && finder.numCoreApisRunning == 0 {
					fmt.Printf("All IPs tested.\n\n")
					fmt.Printf("Successful RPCs: \"%v\"\n\n", strings.Join(finder.successfulRpcs, ","))
					fmt.Printf("Successful Peers: \"%v\"\n\n", strings.Join(finder.successfulPeers, ","))
					fmt.Printf("Successful Core APIs: \"%v\"\n\n", strings.Join(finder.successfulCoreApis, ","))
					if finder.stateSync {
						finder.generateStateSyncConfig()
					}
					finder.stopChan <- syscall.SIGTERM
				}
				finder.mu.Unlock()
			case peer := <-finder.toCheck:
				// Check RPC
				go finder.callRpc(fmt.Sprintf("http://%v:26657", peer.RemoteIP))

				// Check peer better version will attempt to connect to peer
				go finder.dialPeer(peer)

				// Call Vega Core API
				go finder.callCoreApi(fmt.Sprintf("%v:3002", peer.RemoteIP))
			}
		}
	}()

	rpcs := strings.Split(finder.initialRpcs, ",")
	for _, rpcAddr := range rpcs {
		log.Printf("RPC Address: %v\n", rpcAddr)
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
			log.Printf("Finder ChainId: %v\n", finder.chainId)
			log.Printf("Peer ChainID: %v\n", peer.NodeInfo.Network)
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
		finder.failedPeers = append(finder.failedPeers, peerAddr)
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

func (finder *finder) callCoreApi(addr string) {

	finder.mu.Lock()
	finder.numCoreApisRunning++
	finder.mu.Unlock()

	conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		// log.Printf("Could not open connection to core node (%v): %v", addr, err)
		finder.mu.Lock()
		finder.numCoreApisRunning--
		finder.numCoreApisFinished++
		finder.failedCoreApis = append(finder.failedCoreApis, addr)
		finder.mu.Unlock()
		return
	}

	grpcClient := vegaApiPb.NewCoreServiceClient(conn)

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(time.Second*5))
	defer cancel()
	_, err = grpcClient.LastBlockHeight(ctx, &vegaApiPb.LastBlockHeightRequest{})
	if err != nil {
		// log.Printf("Could not get last block from url: %v. Error: %v", addr, err)
		finder.mu.Lock()
		finder.numCoreApisRunning--
		finder.numCoreApisFinished++
		finder.failedCoreApis = append(finder.failedCoreApis, addr)
		finder.mu.Unlock()
		conn.Close()
		return
	}

	// log.Printf("Successful response from core API. Last block height: %v\n", res.Height)

	finder.mu.Lock()
	finder.numCoreApisRunning--
	finder.numCoreApisFinished++
	finder.successfulCoreApis = append(finder.successfulCoreApis, addr)
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
		log.Fatalf("Failed to read response body: %v\n", err)
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
