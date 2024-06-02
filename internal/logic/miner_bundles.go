package logic

import (
	"context"
	"fmt"
	systemix "github.com/gagliardetto/solana-go/programs/system"
	"github.com/mr-tron/base58"
	"math/big"
	"math/rand"
	"sync"
	"time"

	"solxen-tx/internal/logic/generated/sol_xen_miner"

	"github.com/ethereum/go-ethereum/common"
	"github.com/gagliardetto/solana-go"
	computebudget "github.com/gagliardetto/solana-go/programs/compute-budget"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/zeromicro/go-zero/core/errorx"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/mr"
)

var kindRand = rand.New(rand.NewSource(time.Now().UnixNano()))

var bundleInitOnce sync.Once

func (l *Producer) BundlesMiner() error {
	bundleInitOnce.Do(InitBundle)
	var (
		fns []func() error
	)
	ethAccount := common.HexToAddress(l.svcCtx.Config.Sol.ToAddr)
	var uint8Array [20]uint8
	copy(uint8Array[:], ethAccount[:])
	eth := sol_xen_miner.EthAccount{}
	eth.Address = uint8Array
	eth.AddressStr = ethAccount.String()

	// out := make([]rpc.PriorizationFeeResult, 0)
	// feeAccount := []solana.PublicKey{
	// 	solana.MustPublicKeyFromBase58(l.svcCtx.Config.Sol.ProgramId),
	// }

	// fee := l.svcCtx.Config.Sol.Fee
	// if fee == 0 {
	// 	out, _ = l.svcCtx.SolCli.GetRecentPrioritizationFees(l.ctx, feeAccount)
	// 	var feeFata []float64
	// 	for _, item := range out {
	// 		feeFata = append(feeFata, float64(item.PrioritizationFee))
	// 	}
	// 	_fee, _ := stats.Mean(feeFata)
	// 	fee = uint64(_fee) * 1_000_000
	// }

	// set jito bundle tips min (uint64(JitoRealTimeTips.P50Landed*1e9)+1, l.svcCtx.Config.Sol.Fee)
	jitoFee := l.svcCtx.Config.Sol.Fee

	if uint64(JitoRealTimeTips.P50Landed*1e9)+1 < jitoFee {
		jitoFee = uint64(JitoRealTimeTips.P50Landed*1e9) + 1
	}

	for _index, _account := range l.svcCtx.AddrList {
		account := _account
		index := _index
		kind := index % 4

		kind = l.svcCtx.Config.Sol.Kind
		if kind == -1 {
			// optimize: distribute the kind.
			//kind = kindRand.Intn(4)
			// 谨慎改成随机，在提交速度很快的时候怀疑会出问题
			kind = index % 4
		} else {
			account = l.svcCtx.AddrList[0]
		}

		fns = append(fns, func() error {

			t := time.Now()
			var (
				err                error
				globalXnRecordPda  solana.PublicKey
				userEthXnRecordPda solana.PublicKey
				userSolXnRecordPda solana.PublicKey
			)
			mr.Finish(
				func() error {
					globalXnRecordPda, _, err = solana.FindProgramAddress(
						[][]byte{
							[]byte("xn-miner-global"),
							{uint8(kind)},
						},
						l.ProgramIdMiner[kind])
					if err != nil {
						return errorx.Wrap(err, "global_xn_record_pda")
					}
					return nil
				},
				func() error {
					var (
						fromAddr string
					)
					if common.IsHexAddress(l.svcCtx.Config.Sol.ToAddr) {
						fromAddr = l.svcCtx.Config.Sol.ToAddr[2:]
					}

					userEthXnRecordPda, _, err = solana.FindProgramAddress(
						[][]byte{
							[]byte("xn-by-eth"),
							common.FromHex(fromAddr),
							{uint8(kind)},
							l.ProgramIdMiner[kind].Bytes(),
						},
						l.ProgramIdMiner[kind])
					if err != nil {
						return errorx.Wrap(err, "userEthXnRecordAccount")
					}
					return nil
				},
				func() error {

					userSolXnRecordPda, _, err = solana.FindProgramAddress(
						[][]byte{
							[]byte("xn-by-sol"),
							account.PublicKey().Bytes(),
							{uint8(kind)},
							l.ProgramIdMiner[kind].Bytes(),
						},
						l.ProgramIdMiner[kind])
					if err != nil {
						return errorx.Wrap(err, "global_xn_record_pda")
					}

					return nil
				},
			)

			mintToken := sol_xen_miner.NewMineHashesInstruction(
				eth,
				uint8(kind),
				globalXnRecordPda,
				userEthXnRecordPda,
				userSolXnRecordPda,
				account.PublicKey(),
				solana.SystemProgramID,
			).Build()

			// l.svcCtx.Lock.Lock()
			// sol_xen_miner.SetProgramID(ProgramIdMiner[kind])
			data, _ := mintToken.Data()
			instruction := solana.NewInstruction(l.ProgramIdMiner[kind], mintToken.Accounts(), data)
			// l.svcCtx.Lock.Unlock()

			recent, err := l.svcCtx.SolCli.GetLatestBlockhash(context.Background(), rpc.CommitmentFinalized)
			if err != nil {
				return errorx.Wrap(err, "network.")
			}
			rent := recent.Value.Blockhash
			// 生成普通的tx 4个， jito bundles最多支持5个
			bundleSignatures := []string{}
			for i := 0; i < 4; i++ {
				// 防止生成的tx hash一样。
				limit := computebudget.NewSetComputeUnitLimitInstruction(1150000 + uint32(i)).Build()

				//feesInit := computebudget.NewSetComputeUnitPriceInstructionBuilder().SetMicroLamports(uint64(i)).Build()
				tx, err := solana.NewTransactionBuilder().
					//AddInstruction(feesInit).
					AddInstruction(limit).
					AddInstruction(instruction).
					SetRecentBlockHash(rent).
					SetFeePayer(account.PublicKey()).
					Build()
				if err != nil {
					return errorx.Wrap(err, "tx")
				}

				txString, err := txToString(tx, account)
				if err != nil {
					return errorx.Wrap(err, "txString")
				}
				bundleSignatures = append(bundleSignatures, txString)
			}

			// 最后一个 加bundles fee
			//feesInit := computebudget.NewSetComputeUnitPriceInstructionBuilder().SetMicroLamports(0).Build()
			jitoFeesInit := systemix.NewTransferInstructionBuilder().SetFundingAccount(account.PublicKey()).SetRecipientAccount(
				GetTipAddress()).SetLamports(
				jitoFee).Build()
			limit := computebudget.NewSetComputeUnitLimitInstruction(1150000).Build()
			feetx, err := solana.NewTransactionBuilder().
				//AddInstruction(feesInit).
				AddInstruction(limit).
				AddInstruction(instruction).
				AddInstruction(jitoFeesInit).
				SetRecentBlockHash(rent).
				SetFeePayer(account.PublicKey()).
				Build()

			if err != nil {
				return errorx.Wrap(err, "tx")
			}
			txString, err := txToString(feetx, account)
			if err != nil {
				return errorx.Wrap(err, "txString")
			}
			bundleSignatures = append(bundleSignatures, txString)

			var (
				userAccountDataRaw    sol_xen_miner.UserEthXnRecord
				userSolAccountDataRaw sol_xen_miner.UserSolXnRecord
			)
			err = mr.Finish(
				func() error {
					resp, err := sendBundle(bundleSignatures)
					logx.Infof("jito bundle id: %v", resp)
					if err != nil {
						return errorx.Wrap(err, "sig")
					}

					return nil
				},

				func() error {
					err = l.svcCtx.SolCli.GetAccountDataInto(
						l.ctx,
						userEthXnRecordPda,
						&userAccountDataRaw,
					)
					if err != nil {
						// logx.Infof("userAccountDataRaw:%v", err)
						return nil
					}
					return nil
				},

				func() error {
					err = l.svcCtx.SolCli.GetAccountDataInto(
						l.ctx,
						userSolXnRecordPda,
						&userSolAccountDataRaw,
					)
					if err != nil {
						// logx.Infof("userSolAccountDataRaw:%v", err)
						return nil
					}
					return nil
				},
			)
			if err != nil {
				return err
			}

			logx.Infof("account:%v jito fee:%v slot:%v kind:%v hashs:%v superhashes:%v Points:%v tx count:%v t:%v",
				account.PublicKey(),
				jitoFee,
				recent.Context.Slot,
				kind,
				// common.Bytes2Hex(maybe_user_account_data_raw.Nonce[:]),
				userAccountDataRaw.Hashes,
				userAccountDataRaw.Superhashes,
				big.NewInt(0).Div(userSolAccountDataRaw.Points.BigInt(), big.NewInt(1_000_000_000)),
				len(bundleSignatures),
				time.Since(t))

			return nil

		})
	}
	err := mr.Finish(fns...)
	if err != nil {
		logx.Errorf("err: %v", err)
	}
	return nil

}

func txToString(tx *solana.Transaction, account *solana.Wallet) (string, error) {
	signers := []solana.PrivateKey{account.PrivateKey}
	_, err := tx.Sign(
		func(key solana.PublicKey) *solana.PrivateKey {
			for _, signer := range signers {
				if signer.PublicKey().Equals(key) {
					return &signer
				}
			}
			return nil
		},
	)

	txData, err := tx.MarshalBinary()
	if err != nil {
		return "", errorx.Wrap(fmt.Errorf("send transaction: encode transaction: %w", err), "tx err")
	}
	return base58.Encode(txData), nil
}
