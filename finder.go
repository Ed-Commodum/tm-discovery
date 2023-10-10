package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"

	// "log"
	"net/http"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"

	rpcTypes "github.com/cometbft/cometbft/rpc/core/types"
)

type finder struct {
	mu sync.Mutex

	chainId     string
	initialRpcs string

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
	return &finder{
		mu:              sync.Mutex{},
		chainId:         config.ChainId,
		initialRpcs:     config.InitialRpcs,
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
				if finder.numRpcsRunning == 0 && finder.numPeersRunning == 0 {
					fmt.Printf("All IPs tested.\n")
					fmt.Printf("Successful RPCs: \"%v\"\n", strings.Join(finder.successfulRpcs, ","))
					fmt.Printf("Successful Peers: \"%v\"\n", strings.Join(finder.successfulPeers, ","))
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

	decoded := &jsonResponse{}
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
		finder.mu.Unlock()

		finder.mu.Lock()
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

	peerAddr := string(peer.NodeInfo.DefaultNodeID) + peer.RemoteIP + ":26656"
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
