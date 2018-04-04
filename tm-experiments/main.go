package main

import (
	"github.com/tendermint/tendermint/rpc/client"
	"github.com/tendermint/tmlibs/log"
	"github.com/tendermint/tendermint/types"
	"fmt"
	"math/rand"
	"encoding/binary"
	"os"
	"sync"
	"time"
)

var logger = log.NewNopLogger()
var finishedTasks = 0
var mutex = &sync.Mutex{}

func main() {

	var endpoint = "tcp://0.0.0.0:46657"

	var httpClient = getHTTPClient(endpoint)

	var res, err = httpClient.Status()
	if err != nil {
		logger.Info("something wrong happens", err)
	}
	logger.Info("received status", res)

	go monitorTask(endpoint)

	txCount := 10
	var clientNumber = 10
	for i := 0; i < clientNumber; i++ {
		go clientTask(i, txCount, endpoint)
	}
	for finishedTasks < clientNumber+1 {}
	fmt.Printf("Done: %d\n", finishedTasks)
}

func clientTask(id, txCount int, endpoint string) {
	var httpClient = getHTTPClient(endpoint)
	for i := 0; i < txCount; i++ {
		var _, err = httpClient.BroadcastTxSync(generateTx(id, rand.Int()))
		if err != nil {
			fmt.Printf("Something wrong happened: %s\n", err)
		}
	}
	fmt.Printf("Finished client task: %d\n", id)

	mutex.Lock()
	finishedTasks ++
	mutex.Unlock()
}

func getHTTPClient(rpcAddr string) *client.HTTP {
	return client.NewHTTP(rpcAddr, "/websocket")
}

func generateTx(i, valI int) []byte {
	// a tx encodes the validator index, the tx number, and some random junk
	tx := make([]byte, 250)
	binary.PutUvarint(tx[:32], uint64(valI))
	binary.PutUvarint(tx[32:64], uint64(i))
	if _, err := rand.Read(tx[65:]); err != nil {
		fmt.Println("err reading from crypto/rand", err)
		os.Exit(1)
	}
	return tx
}

func monitorTask(endpoint string) {
	var waitForEventTimeout = 5 * time.Second

	var httpClient = getHTTPClient(endpoint)
	httpClient.Start()

	evtTyp := types.EventNewBlockHeader
	evt, err := client.WaitForOneEvent(httpClient, evtTyp, waitForEventTimeout)
	if err != nil {
		fmt.Println("error when waiting for header", err)
	} else {
		header, ok := evt.Unwrap().(types.EventDataNewBlockHeader)
		if !ok {
			fmt.Println("received header", header)
		}
	}

	fmt.Printf("Finished monitor task\n")

	mutex.Lock()
	finishedTasks ++
	mutex.Unlock()
}




