package logic

import (
	"context"
	systemix "github.com/gagliardetto/solana-go/programs/system"
	"solxen-tx/internal/logic/generated/sol_xen_minter"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	computebudget "github.com/gagliardetto/solana-go/programs/compute-budget"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/zeromicro/go-zero/core/errorx"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/mr"
)

func (l *Producer) Mint() error {

	var (
		fns []func() error
		// programIdMiner = solana.MustPublicKeyFromBase58(l.svcCtx.Config.Sol.ProgramIdMiner)
	)

	// limit := computebudget.NewSetComputeUnitLimitInstruction(1400000).Build()
	feesInit := computebudget.NewSetComputeUnitPriceInstructionBuilder().SetMicroLamports(l.svcCtx.Config.Sol.Fee).Build()

	for _index, _account := range l.svcCtx.AddrList {
		account := _account
		index := _index
		kind := index % 4

		kind = l.svcCtx.Config.Sol.Kind
		if kind == -1 {
			kind = index % 4
		} else if kind == -2 {
		} else {
			account = l.svcCtx.AddrList[0]
		}

		fns = append(fns, func() error {
			// t := time.Now()

			instruction, user_token_record_pda := l.genMinterInstruct(account, kind)

			signers := []solana.PrivateKey{account.PrivateKey}
			recent, err := l.svcCtx.SolCli.GetLatestBlockhash(context.Background(), rpc.CommitmentFinalized)
			rent := recent.Value.Blockhash

			if l.svcCtx.Config.Sol.EnableJitoBundles {
				bundleSignatures := []string{}

				jitoFeesInx := systemix.NewTransferInstructionBuilder().SetFundingAccount(account.PublicKey()).SetRecipientAccount(
					GetTipAddress()).SetLamports(
					l.svcCtx.Config.Sol.Fee).Build()
				limit := computebudget.NewSetComputeUnitLimitInstruction(1360000).Build()

				// kind != -2 minter 单个kind
				tx, err := solana.NewTransactionBuilder().
					AddInstruction(limit).
					AddInstruction(instruction).
					AddInstruction(jitoFeesInx).
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

				if kind == -2 {
					// -2 要minter 所有 kind 0-3
					bundleSignatures = []string{}
					for k := 0; k < 3; k++ {
						instruction, _ := l.genMinterInstruct(account, k)
						tx, err := solana.NewTransactionBuilder().
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
					instruction, _ := l.genMinterInstruct(account, 3)
					tx, err := solana.NewTransactionBuilder().
						AddInstruction(limit).
						AddInstruction(instruction).
						AddInstruction(jitoFeesInx).
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

				resp, err := sendBundle(bundleSignatures)
				logx.Infof("jito bundle id: %v", resp)
				if err != nil {
					return errorx.Wrap(err, "sig")
				}

			} else {
				if kind == -2 {
					panic("not support kind = -2 without jito bundles")
				}
				tx, err := solana.NewTransactionBuilder().
					AddInstruction(feesInit).
					// AddInstruction(limit).
					AddInstruction(instruction).
					SetRecentBlockHash(rent).
					SetFeePayer(account.PublicKey()).
					Build()
				if err != nil {
					return errorx.Wrap(err, "tx")
				}

				_, err = tx.Sign(
					func(key solana.PublicKey) *solana.PrivateKey {
						for _, signer := range signers {
							if signer.PublicKey().Equals(key) {
								return &signer
							}
						}
						return nil
					},
				)
				if err != nil {
					return errorx.Wrap(err, "Sign")
				}

				// logx.Infof("tx :%v", tx)

				_, err = l.svcCtx.SolCli.SendTransactionWithOpts(context.Background(), tx, rpc.TransactionOpts{
					SkipPreflight: false,
				})
				if err != nil {
					// logx.Errorf("accout:%v : no token info", account.PublicKey())
					// return errorx.Wrap(err, "sig")
				}
			}

			var (
				user_balance_data_raw sol_xen_minter.UserTokensRecord
			)
			// err = l.svcCtx.SolCli.GetAccountDataInto(l.ctx, user_token_record_pda, &user_balance_data_raw)
			// if err != nil {
			//
			// }
			resp, err := l.svcCtx.SolCli.GetAccountInfoWithOpts(l.ctx, user_token_record_pda, &rpc.GetAccountInfoOpts{
				Commitment: rpc.CommitmentConfirmed,
			})
			if err != nil {
				// return err
			} else {
				err = bin.NewBinDecoder(resp.Value.Data.GetBinary()).Decode(&user_balance_data_raw)
				if err != nil {
					// return err
				}
			}

			logx.Infof("account:%v tokens:%v ",
				account.PublicKey(),
				user_balance_data_raw.TokensMinted,
			)

			return nil

		})
	}

	err := mr.Finish(fns...)
	if err != nil {
		logx.Errorf("err: %v", err)
	}
	return nil
}

func (l *Producer) genMinterInstruct(account *solana.Wallet, kind int) (solana.Instruction, solana.PublicKey) {
	if kind == -2 {
		kind = 0
	}
	programId := solana.MustPublicKeyFromBase58(l.svcCtx.Config.Sol.ProgramId)

	mint_pda, _, err := solana.FindProgramAddress(
		[][]byte{
			[]byte("mint"),
		},
		programId,
	)
	if err != nil {
		panic(errorx.Wrap(err, "mint_pda"))
	}
	// spew.Dump(mint_pda)
	user_sol_xn_record_pda, _, err := solana.FindProgramAddress(
		[][]byte{
			[]byte("xn-by-sol"),
			account.PublicKey().Bytes(),
			{uint8(kind)},
			l.ProgramIdMiner[kind].Bytes(),
		},
		l.ProgramIdMiner[kind],
	)
	if err != nil {
		panic(errorx.Wrap(err, "userSolXnRecordPda"))
	}
	// spew.Dump(user_sol_xn_record_pda)

	user_token_record_pda, _, err := solana.FindProgramAddress(
		[][]byte{
			[]byte("sol-xen-minted"),
			account.PublicKey().Bytes(),
		},
		programId,
	)
	if err != nil {
		panic(errorx.Wrap(err, "user_eth_xn_record_pda"))
	}

	// spew.Dump(user_token_record_pda)

	associate_token_program := solana.SPLAssociatedTokenAccountProgramID
	user_token_account, _, err := solana.FindAssociatedTokenAddress(account.PublicKey(), mint_pda)
	// spew.Dump(user_token_account)

	mintToken := sol_xen_minter.NewMintTokensInstruction(
		uint8(kind),
		user_sol_xn_record_pda,
		user_token_record_pda,
		user_token_account,
		account.PublicKey(),
		mint_pda,
		solana.TokenProgramID,
		solana.SystemProgramID,
		associate_token_program,
		l.ProgramIdMiner[kind],
	).Build()

	// sol_xen_minter.SetProgramID(solana.MustPublicKeyFromBase58(l.svcCtx.Config.Sol.ProgramId))

	data, _ := mintToken.Data()
	instruction := solana.NewInstruction(solana.MustPublicKeyFromBase58(l.svcCtx.Config.Sol.ProgramId), mintToken.Accounts(), data)
	return instruction, user_token_record_pda
}
