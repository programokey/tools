package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	cmn "github.com/tendermint/tmlibs/common"
	"github.com/tendermint/tmlibs/log"
	"github.com/programokey/tools/tm-monitor/monitor"
	"bytes"
)

var version = "0.4.0"

var logger = log.NewNopLogger()

func main() {
	var listenAddr string
	var noton bool

	flag.StringVar(&listenAddr, "listen-addr", "tcp://0.0.0.0:46670", "HTTP and Websocket server listen address")
	flag.BoolVar(&noton, "no-ton", false, "Do not show ton (table of nodes)")

	flag.Usage = func() {
		fmt.Println(`Tendermint monitor watches over one or more Tendermint core
applications, collecting and providing various statistics to the user.

Usage:
	tm-monitor [-no-ton] [-listen-addr="tcp://0.0.0.0:46670"] [endpoints]

Examples:
	# monitor single instance
	tm-monitor localhost:46657

	# monitor a few instances by providing comma-separated list of RPC endpoints
	tm-monitor host1:46657,host2:46657`)
		fmt.Println("Flags:")
		flag.PrintDefaults()
	}

	flag.Parse()

	if flag.NArg() == 0 {
		flag.Usage()
		os.Exit(1)
	}

	////第二个参数为no-ton
	//b, err := strconv.ParseBool(flag.Arg(1))
	//if err != nil{
	//
	//}else{
	//	fmt.Println("non-ton",flag.Arg(1))
	//}
	//
	//noton = b

	if noton {
		logger = log.NewTMLogger(log.NewSyncWriter(os.Stdout))
	}

	var endpoints_buffer bytes.Buffer
	for _, arg := range flag.Args(){
		endpoints_buffer.WriteString(strings.TrimSpace(arg))
	}

	endpoints := endpoints_buffer.String()

	m := startMonitor(endpoints)

	startRPC(listenAddr, m, logger)

	var ton *Ton
	if !noton {
		ton = NewTon(m)
		ton.Start()
	}

	cmn.TrapSignal(func() {
		if !noton {
			ton.Stop()
		}
		m.Stop()
	})
}

func startMonitor(endpoints string) *monitor.Monitor {
	m := monitor.NewMonitor()
	m.SetLogger(logger.With("component", "monitor"))

	for _, e := range strings.Split(endpoints, ",") {
		n := monitor.NewNode(e)
		n.SetLogger(logger.With("node", e))
		if err := m.Monitor(n); err != nil {
			//panic(err)
			//如果无法连接，不要panic
			fmt.Errorf("err connect to ",endpoints)
		}
	}

	if err := m.Start(); err != nil {
		panic(err)
	}

	return m
}
