package logic

import (
	"context"
	"solxen-tx/internal/svc"
	"sync"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/zeromicro/go-zero/core/logx"
)

type Producer struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
	all            int
	mux            sync.RWMutex
	ProgramIdMiner solana.PublicKeySlice
}

func NewProducerLogic(ctx context.Context, svcCtx *svc.ServiceContext) *Producer {
	return &Producer{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
		all:    0,
		mux:    sync.RWMutex{},
		ProgramIdMiner: solana.PublicKeySlice{
			solana.MustPublicKeyFromBase58("B8HwMYCk1o7EaJhooM4P43BHSk5M8zZHsTeJixqw7LMN"),
			solana.MustPublicKeyFromBase58("2Ewuie2KnTvMLwGqKWvEM1S2gUStHzDUfrANdJfu45QJ"),
			solana.MustPublicKeyFromBase58("5dxcK28nyAJdK9fSFuReRREeKnmAGVRpXPhwkZxAxFtJ"),
			solana.MustPublicKeyFromBase58("DdVCjv7fsPPm64HnepYy5MBfh2bNfkd84Rawey9rdt5S"),

			// eat ,,,
			// solana.MustPublicKeyFromBase58("CFRDmC2xPN7K2D8GadHKpcwSAC5YvPzPjbjYA6v439oi"),
			// solana.MustPublicKeyFromBase58("7vQ9pG7MUjkswNkL96XiiYbz3swM9dkqgMEAbgDaLggi"),
			// solana.MustPublicKeyFromBase58("DpLx72BXVhZN6hkA6LKKres3EUKvK36mmh5JaKyaVSYU"),
			// solana.MustPublicKeyFromBase58("7u5D7qPHGZHXQ3nQTeZu5eFKtKGKQWKhJCdM1B3T4Ly4"),
		},
	}
}

func (l *Producer) Start() {
	logx.Infof("start  miner")

	// var subscription pb.SubscribeRequest
	// subscription.Blocks = make(map[string]*pb.SubscribeRequestFilterBlocks)
	// subscription.Blocks["blocks"] = &pb.SubscribeRequestFilterBlocks{}
	//
	// subscriptionJson, err := json.Marshal(&subscription)
	// if err != nil {
	// 	logx.Errorf("Failed to marshal subscription request: %v", subscriptionJson)
	// }
	// logx.Infof("Subscription request: %s", string(subscriptionJson))
	//
	// stream, err := l.svcCtx.GrpcCli.Subscribe(context.Background())
	// if err != nil {
	// 	log.Fatalf("%v", err)
	// }
	// err = stream.Send(&subscription)
	// if err != nil {
	// 	log.Fatalf("%v", err)
	// }

	// for {
	// 	resp, err := stream.Recv()
	// 	// timestamp := time.Now().UnixNano()
	// 	if err == io.EOF {
	// 		return
	// 	}
	// 	if err != nil {
	// 		log.Fatalf("Error occurred in receiving update: %v", err)
	// 	}
	// 	block := resp.GetBlock()
	// 	// log.Printf("timestamp: %v block:%v %v %v", timestamp, block.GetBlockhash(), block.GetSlot(), block.GetBlockHeight())
	// 	Blockhash := block.GetBlockhash()
	// 	slot := block.GetSlot()
	// 	if Blockhash == "" || slot == 0 {
	// 		continue
	// 	}
	//
	// 	err = l.Mint()
	// 	if err != nil {
	// 		logx.Errorf("Mint err:%v", err)
	// 		continue
	// 	}
	//
	// }

	for {
		// 1. CheckAddressBalance
		err := l.CheckAddressBalance()
		if err != nil {
			logx.Errorf("%v", err)
			return
		}
		// todo 2.QueryNetWorkGas
		// err = l.QueryNetWorkGas()
		// if err != nil {
		// 	logx.Errorf("%v", err)
		// 	return
		// }

		// 3.Miner
		if l.svcCtx.Config.Sol.EnableJitoBundles {
			err = l.BundlesMiner()
			if err != nil {
				logx.Errorf("Mint err:%v", err)
				continue
			}
		} else {
			err = l.Miner()
			if err != nil {
				logx.Errorf("Mint err:%v", err)
				continue
			}
		}

		time.Sleep(time.Duration(l.svcCtx.Config.Sol.Time) * time.Millisecond)
	}

}

func (l *Producer) Stop() {
	logx.Infof("stop Producer \n")
}
