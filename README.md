# Tendermint RPC and Peer Discovery Tool

This tool discovers TM RPCs and peer addresses on Tendermint/CometBFT networks by checking the default TM RPC port on all public remote IPs reported by the net_info endpoint. 


## Usage


```
git pull https://github.com/Ed-Commodum/tm-discovery.git
cd vega-discovery
go build -o tm-discover
./tm-discover
```

You can privide a comma separated list of RPCs to search through like so:
```
./tm-discover --initial-rpcs="http://foo.bar/rpc,http://111.222.333.444:26657"
```
When providing initial RPCs on a network other than the default (vega-mainnet-0011), you will either need to set the chain ID with the --chain-id flag, or use the --infer-chain-id flag. Mixing RPCs from multiple networks will lead to an error and will terminate the program.
```
./tm-discover --initial-rpcs="http://foo.bar/rpc,http://111.222.333.444:26657" --chain-id="bitconnect-mainnet"
```
or
```
./tm-discover --initial-rpcs="http://foo.bar/rpc,http://111.222.333.444:26657" --infer-chain-id
```

You can use the --state-sync flag to generate statesync config which will be printed in the terminal

```
./tm-discover --state-sync
```