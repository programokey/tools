package monitor

import (
	"encoding/json"
	"math"
	"time"

	"github.com/pkg/errors"
	"github.com/tendermint/go-crypto"
	ctypes "github.com/tendermint/tendermint/rpc/core/types"
	rpc_client "github.com/tendermint/tendermint/rpc/lib/client"
	tmtypes "github.com/tendermint/tendermint/types"
	"github.com/tendermint/tmlibs/events"
	"github.com/tendermint/tmlibs/log"
	em "github.com/programokey/tools/tm-monitor/eventmeter"
	persistent "github.com/programokey/tools/tm-monitor/persistent"

)

const maxRestarts = 25

type Node struct {
	rpcAddr string

	IsValidator bool          `json:"is_validator"` // validator or non-validator?

	Power    int64		`json:"power"`
	PowerRatio  int       `json:"power_ratio"`
	pubKey      crypto.PubKey `json:"pub_key"`

	PrecommitSum int `json:"pc_sum"`
	PrecommitMiss int `json:"pc_miss"`

	Name         string  `json:"name"`
	Online       bool    `json:"online"`
	Height       int64   `json:"height"`
	BlockLatency float64 `json:"block_latency" amino:"unsafe"` // ms, interval between block commits

	// em holds the ws connection. Each eventMeter callback is called in a separate go-routine.
	em eventMeter

	// rpcClient is an client for making RPC calls to TM
	rpcClient rpc_client.HTTPClient

	blockCh        chan<- tmtypes.Header
	//收到的完整区块
	fullblockCh        chan<- tmtypes.Block
	blockLatencyCh chan<- float64
	disconnectCh   chan<- bool

	persistent 	*persistent.Persistent

	checkIsValidatorInterval time.Duration

	quit chan struct{}

	logger log.Logger
}

func NewNode(rpcAddr string, options ...func(*Node)) *Node {
	em := em.NewEventMeter(rpcAddr, UnmarshalEvent)
	rpcClient := rpc_client.NewURIClient(rpcAddr) // HTTP client by default
	rpcClient.SetCodec(cdc)
	return NewNodeWithEventMeterAndRpcClient(rpcAddr, em, rpcClient, options...)
}

func NewNodeWithEventMeterAndRpcClient(rpcAddr string, em eventMeter, rpcClient rpc_client.HTTPClient, options ...func(*Node)) *Node {
	n := &Node{
		rpcAddr:   rpcAddr,
		em:        em,
		rpcClient: rpcClient,
		Name:      rpcAddr,
		quit:      make(chan struct{}),
		checkIsValidatorInterval: 5 * time.Second,
		logger: log.NewNopLogger(),
	}

	for _, option := range options {
		option(n)
	}

	n.persistent = persistent.NewPersistent("tm-monitor", "block")
	n.persistent.Index([]string{"height"})
	n.persistent.Index([]string{"time"})
	return n
}

// SetCheckIsValidatorInterval lets you change interval for checking whenever
// node is still a validator or not.
func SetCheckIsValidatorInterval(d time.Duration) func(n *Node) {
	return func(n *Node) {
		n.checkIsValidatorInterval = d
	}
}

//添加fullblock传送
func (n *Node) SendFullBlocksTo(ch chan<- tmtypes.Block) {
	n.fullblockCh = ch
}

func (n *Node) SendBlocksTo(ch chan<- tmtypes.Header) {
	n.blockCh = ch
}

func (n *Node) SendBlockLatenciesTo(ch chan<- float64) {
	n.blockLatencyCh = ch
}

func (n *Node) NotifyAboutDisconnects(ch chan<- bool) {
	n.disconnectCh = ch
}

// SetLogger lets you set your own logger
func (n *Node) SetLogger(l log.Logger) {
	n.logger = l
	n.em.SetLogger(l)
}

func (n *Node) Start() error {
	if err := n.em.Start(); err != nil {
		return err
	}

	n.em.RegisterLatencyCallback(latencyCallback(n))
	//添加callback
	err := n.em.Subscribe(tmtypes.EventQueryNewBlockHeader.String(), newBlockCallback(n))
	if err != nil {
		return err
	}
	//添加full block的callback
	err = n.em.Subscribe(tmtypes.EventQueryNewBlock.String(), newFullBlockCallback(n))
	if err != nil {
		return err
	}

	/*
	EventQueryBond              = QueryForEvent(EventBond)
	EventQueryUnbond            = QueryForEvent(EventUnbond)
	EventQueryRebond            = QueryForEvent(EventRebond)
	EventQueryDupeout           = QueryForEvent(EventDupeout)
	EventQueryFork              = QueryForEvent(EventFork)
	EventQueryNewBlock          types.EventDataNewBlock
	EventQueryNewBlockHeader    types.EventDataNewBlockHeader
	EventQueryNewRound          types.EventDataRoundState
	EventQueryNewRoundStep      types.EventDataRoundState
	EventQueryTimeoutPropose    types.EventDataRoundState
	EventQueryCompleteProposal  types.EventDataRoundState
	EventQueryPolka             types.EventDataRoundState
	EventQueryUnlock            = QueryForEvent(EventUnlock)
	EventQueryLock              = QueryForEvent(EventLock)
	EventQueryRelock            = QueryForEvent(EventRelock)
	EventQueryTimeoutWait       = QueryForEvent(EventTimeoutWait)
	EventQueryVote              types.EventDataVote
	EventQueryProposalHeartbeat types.EventDataProposalHeartbeat
	EventQueryTx                types.EventDataTx
	*/

	n.em.RegisterDisconnectCallback(disconnectCallback(n))

	n.Online = true

	n.checkIsValidator()
	go n.checkIsValidatorLoop()

	return nil
}

func (n *Node) Stop() {
	n.Online = false

	n.em.Stop()

	close(n.quit)
}

// implements eventmeter.EventCallbackFunc
func newBlockCallback(n *Node) em.EventCallbackFunc {
	return func(metric *em.EventMetric, data interface{}) {
		block := data.(tmtypes.TMEventData).(tmtypes.EventDataNewBlockHeader).Header

		n.Height = block.Height
		n.logger.Info("new block", "height", block.Height, "numTxs", block.NumTxs)

		if n.blockCh != nil {
			n.blockCh <- *block
		}
	}
}

// implements eventmeter.EventCallbackFunc
func newFullBlockCallback(n *Node) em.EventCallbackFunc {
	return func(metric *em.EventMetric, data interface{}) {

		block := data.(tmtypes.TMEventData).(tmtypes.EventDataNewBlock).Block

		n.Height = block.Height
		n.persistent.Save(block)

		n.logger.Info("new block", "height", block.Height, "hash", block.LastCommit.Hash())

		// return nil if n isn't a validator
		if n.pubKey == nil || !n.IsValidator{
			return
		}
		pc := block.LastCommit.Precommits
		voteNil := true
		for _,commit:=range pc {
			if pc != nil && commit != nil&& n.pubKey!= nil {
				if n.pubKey.Address().String()==commit.ValidatorAddress.String() {
					n.PrecommitSum ++
					voteNil = false
				}
			}
		}

		if voteNil {
			n.PrecommitMiss ++
		}
		if n.fullblockCh != nil {
			n.fullblockCh <- *block
		}
	}
}

// implements eventmeter.EventLatencyFunc
func latencyCallback(n *Node) em.LatencyCallbackFunc {
	return func(latency float64) {
		n.BlockLatency = latency / 1000000.0 // ns to ms
		n.logger.Info("new block latency", "latency", n.BlockLatency)

		if n.blockLatencyCh != nil {
			n.blockLatencyCh <- latency
		}
	}
}

// implements eventmeter.DisconnectCallbackFunc
func disconnectCallback(n *Node) em.DisconnectCallbackFunc {
	return func() {
		n.Online = false
		n.logger.Info("status", "down")

		if n.disconnectCh != nil {
			n.disconnectCh <- true
		}
	}
}

func (n *Node) RestartEventMeterBackoff() error {
	attempt := 0

	for {
		d := time.Duration(math.Exp2(float64(attempt)))
		time.Sleep(d * time.Second)

		if err := n.em.Start(); err != nil {
			n.logger.Info("restart failed", "err", err)
		} else {
			// TODO: authenticate pubkey
			return nil
		}

		attempt++

		if attempt > maxRestarts {
			return errors.New("Reached max restarts")
		}
	}
}

func (n *Node) NumValidators() (height int64, num int, err error) {
	height, vals, err := n.validators()
	if err != nil {
		return 0, 0, err
	}
	return height, len(vals), nil
}

func (n *Node) validators() (height int64, validators []*tmtypes.Validator, err error) {
	vals := new(ctypes.ResultValidators)
	if _, err = n.rpcClient.Call("validators", nil, vals); err != nil {
		return 0, make([]*tmtypes.Validator, 0), err
	}
	return vals.BlockHeight, vals.Validators, nil
}

//统计每一个块中的precommit
func (n *Node) precommits() (height int64, validators []*tmtypes.Validator, err error) {
	vals := new(ctypes.ResultValidators)
	if _, err = n.rpcClient.Call("validators", nil, vals); err != nil {
		return 0, make([]*tmtypes.Validator, 0), err
	}
	return vals.BlockHeight, vals.Validators, nil
}

func (n *Node) checkIsValidatorLoop() {
	for {
		select {
		case <-n.quit:
			return
		case <-time.After(n.checkIsValidatorInterval):
			n.checkIsValidator()
		}
	}
}

func (n *Node) checkIsValidator() {
	_, validators, err := n.validators()
	if err == nil {
		for _, v := range validators {
			key, err1 := n.getPubKey()
			// TODO: use bytes.Equal
			if err1 == nil && v.PubKey == key {
				n.IsValidator = true
				n.Power = v.VotingPower
			}
		}
	} else {
		n.logger.Info("check is validator failed", "err", err)
	}
}




func (n *Node) getPubKey() (crypto.PubKey, error) {
	if n.pubKey != nil {
		return n.pubKey, nil
	}

	status := new(ctypes.ResultStatus)
	_, err := n.rpcClient.Call("status", nil, status)
	if err != nil {
		return nil, err
	}
	n.pubKey = status.ValidatorInfo.PubKey
	return n.pubKey, nil
}

type eventMeter interface {
	Start() error
	Stop()
	RegisterLatencyCallback(em.LatencyCallbackFunc)
	RegisterDisconnectCallback(em.DisconnectCallbackFunc)
	Subscribe(string, em.EventCallbackFunc) error
	Unsubscribe(string) error
	SetLogger(l log.Logger)
}

// UnmarshalEvent unmarshals a json event
func UnmarshalEvent(b json.RawMessage) (string, events.EventData, error) {
	event := new(ctypes.ResultEvent)
	if err := cdc.UnmarshalJSON(b, event); err != nil {
		return "", nil, err
	}
	return event.Query, event.Data, nil
}



////get total bonded steaks
func (n *Node) GetTotalSteaks() int64{
	sum := int64(0)
	_,vs,err:= n.validators()

	if err != nil {
		return 0
	}

	for _,v := range vs{

		sum+=v.VotingPower

		}

	return sum
}
