package testsuite

import (
	"fmt"
	"math"
	"testing"

	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	"github.com/iotaledger/iota-core/pkg/testsuite/snapshotcreator"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/tpkg"
	"github.com/sajari/regression"
)

// Test_Regression runs benchmarks for many block types and find the best fit regression model.
func Test_Regression(t *testing.T) {
	r := new(regression.Regression)
	r.SetObserved("ns/op")
	r.SetVar(0, "Input")
	r.SetVar(1, "ContextInput")
	r.SetVar(2, "Output")
	r.SetVar(3, "NativeToken")
	r.SetVar(4, "Staking")
	r.SetVar(5, "BlockIssuer")
	r.SetVar(6, "Allotment")
	r.SetVar(7, "SignatureEd25519")

	r.Train(
		regression.DataPoint(basicInNativeOut(t, 1)),
		regression.DataPoint(basicInNativeOut(t, 20)),
		regression.DataPoint(basicInNativeOut(t, iotago.MaxOutputsCount-1)),
		regression.DataPoint(allotments(t, 1)),
		regression.DataPoint(allotments(t, 20)),
		regression.DataPoint(allotments(t, iotago.MaxAllotmentCount)),
		regression.DataPoint(basicInBasicOut(t, 1, 1, false)),
		regression.DataPoint(basicInBasicOut(t, 1, 20, false)),
		regression.DataPoint(basicInBasicOut(t, 1, iotago.MaxOutputsCount, false)),
		regression.DataPoint(basicInBasicOut(t, 20, 1, false)),
		regression.DataPoint(basicInBasicOut(t, 20, 1, true)),
		regression.DataPoint(basicInBasicOut(t, iotago.MaxInputsCount, 1, false)),
		regression.DataPoint(basicInBasicOut(t, iotago.MaxInputsCount, 1, true)),
		regression.DataPoint(accInAccOut(t)),
		//regression.DataPoint(accInAccStakingOut(t)),
		regression.DataPoint(oneInAccOut(t)),
		regression.DataPoint(oneInAccStakingOut(t)),
		regression.DataPoint(oneInAccRemOut(t)),
		regression.DataPoint(oneInAccStakingRemOut(t)),

		// regression.DataPoint(1951938, []float64{5, 0, 1, 0, 0, 0, 0, 1}),
		// regression.DataPoint(2654715, []float64{20, 0, 1, 0, 0, 0, 0, 1}),

		// regression.DataPoint(3839477, []float64{20, 0, 1, 0, 0, 0, 0, 20}),
		// regression.DataPoint(2892937, []float64{5, 0, 1, 0, 0, 0, 0, 5}),

		// regression.DataPoint(2958266, []float64{1, 0, 1, 0, 0, 0, 2, 1}),
		// regression.DataPoint(2943738, []float64{1, 0, 1, 0, 0, 0, 1, 1}),
		// regression.DataPoint(81213422, []float64{2, 2, 128, 126, 0, 1, 0, 1}),
		// regression.DataPoint(4482071, []float64{2, 2, 2, 1, 0, 1, 0, 1}),
		// regression.DataPoint(2710304, []float64{1, 2, 1, 0, 0, 1, 0, 1}),
		// regression.DataPoint(2574392, []float64{1, 1, 1, 0, 0, 1, 0, 1}),
		// regression.DataPoint(2576957, []float64{1, 1, 1, 0, 1, 1, 0, 1}),
		// regression.DataPoint(2659167, []float64{1, 1, 2, 0, 0, 1, 0, 1}),
		// regression.DataPoint(2748479, []float64{1, 1, 2, 0, 1, 1, 0, 1}),
		// regression.DataPoint(74449111, []float64{128, 0, 128, 0, 0, 0, 0, 128}),
		// regression.DataPoint(12534771, []float64{128, 0, 1, 0, 0, 0, 0, 128}),
		// regression.DataPoint(2387996, []float64{1, 0, 1, 0, 0, 0, 0, 1}),
		// regression.DataPoint(47375253, []float64{1, 0, 128, 0, 0, 0, 0, 1}),
	)

	r.Run()
	printCoefficients(r.GetCoeffs())
	fmt.Println(r.Formula)
}

func basicInBasicOut(t *testing.T, numIn int, numOut int, signatures bool) (float64, []float64) {
	blockchan := make(chan *blocks.Block, 1)
	var block *iotago.Block

	// basic block with one input and one output
	fn := func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			b.StopTimer()
			ts := NewTestSuite(t)
			node := ts.AddValidatorNode("node1")
			ts.AddDefaultWallet(node)
			ts.Run(true)
			var addressIndexes []uint32
			for i := 0; i < numIn; i++ {
				addressIndexes = append(addressIndexes, uint32(i))
			}
			// First, create 128 outputs
			var tx1 *iotago.SignedTransaction
			if signatures {
				tx1 = ts.DefaultWallet().CreateBasicOutputsAtAddressesFromInput(
					"tx1",
					addressIndexes,
					"Genesis:0",
				)
			} else {
				tx1 = ts.DefaultWallet().CreateBasicOutputsEquallyFromInput(
					"tx1",
					numIn,
					"Genesis:0",
				)
			}
			genesisCommitment := iotago.NewEmptyCommitment(ts.API)
			genesisCommitment.ReferenceManaCost = ts.API.ProtocolParameters().CongestionControlParameters().MinReferenceManaCost
			block1 := ts.IssueBasicBlockWithOptions("block1", ts.DefaultWallet(), tx1, mock.WithSlotCommitment(genesisCommitment))
			block = block1.ProtocolBlock()
			modelBlock := lo.PanicOnErr(model.BlockFromBlock(block))
			node.Protocol.Events.Engine.Scheduler.BlockScheduled.Hook(func(block *blocks.Block) {
				blockchan <- block
			})
			node.Protocol.IssueBlock(modelBlock)
			<-blockchan

			inputNames := make([]string, numIn)
			for i := 0; i < numIn; i++ {
				inputNames[i] = fmt.Sprintf("tx1:%d", i)
			}
			// Then, create a transaction with 128 inputs
			tx2 := ts.DefaultWallet().CreateBasicOutputsEquallyFromInputs(
				"tx2",
				inputNames,
				addressIndexes,
				numOut,
			)
			genesisCommitment.ReferenceManaCost = ts.API.ProtocolParameters().CongestionControlParameters().MinReferenceManaCost
			block2 := ts.IssueBasicBlockWithOptions("block2", ts.DefaultWallet(), tx2, mock.WithSlotCommitment(genesisCommitment))
			block = block2.ProtocolBlock()
			modelBlock = lo.PanicOnErr(model.BlockFromBlock(block))
			b.StartTimer()
			// time from issuance of the block to when it is scheduled
			node.Protocol.IssueBlock(modelBlock)
			<-blockchan
			b.StopTimer()
			ts.Shutdown()
		}
	}
	// get the ns/op of processing the block
	nsPerBlock := float64(testing.Benchmark(fn).NsPerOp())
	fmt.Printf("Max inputs one output: %f ns/op\n", nsPerBlock)
	// get the regressors
	regressors := GetBlockWorkScoreRegressors(block)
	printRegressors(regressors)

	return nsPerBlock, regressors
}

func oneInAccOut(t *testing.T) (float64, []float64) {
	blockchan := make(chan *blocks.Block, 1)
	var block *iotago.Block

	// basic block with one input, one account output with staking and a remainder
	fn := func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			b.StopTimer()
			ts := NewTestSuite(t)
			node := ts.AddValidatorNode("node1")
			ts.AddDefaultWallet(node)
			ts.Run(true)
			tx1 := ts.DefaultWallet().CreateAccountFromInput(
				"tx1",
				"Genesis:0",
				ts.DefaultWallet(),
				mock.WithBlockIssuerFeature(iotago.BlockIssuerKeys{tpkg.RandBlockIssuerKey()}, iotago.MaxSlotIndex),
				mock.WithAccountMana(mock.MaxBlockManaCost(ts.DefaultWallet().Node.Protocol.CommittedAPI().ProtocolParameters())),
			)
			// default block issuer issues a block containing the transaction in slot 1.
			genesisCommitment := iotago.NewEmptyCommitment(ts.API)
			genesisCommitment.ReferenceManaCost = ts.API.ProtocolParameters().CongestionControlParameters().MinReferenceManaCost
			block1 := ts.IssueBasicBlockWithOptions("block1", ts.DefaultWallet(), tx1, mock.WithSlotCommitment(genesisCommitment))
			block = block1.ProtocolBlock()
			modelBlock := lo.PanicOnErr(model.BlockFromBlock(block))
			node.Protocol.Events.Engine.Scheduler.BlockScheduled.Hook(func(block *blocks.Block) {
				blockchan <- block
			})
			b.StartTimer()
			// time from issuance of the block to when it is scheduled
			node.Protocol.IssueBlock(modelBlock)
			<-blockchan
			b.StopTimer()
			ts.Shutdown()
		}
	}
	// get the ns/op of processing the block
	nsPerBlock := float64(testing.Benchmark(fn).NsPerOp())
	fmt.Printf("One input one account output and remainder: %f ns/op\n", nsPerBlock)
	// get the regressors
	regressors := GetBlockWorkScoreRegressors(block)
	printRegressors(regressors)

	return nsPerBlock, regressors
}

func oneInAccStakingOut(t *testing.T) (float64, []float64) {
	blockchan := make(chan *blocks.Block, 1)
	var block *iotago.Block

	// basic block with one input, one account output with staking and a remainder
	fn := func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			b.StopTimer()
			ts := NewTestSuite(t)
			node := ts.AddValidatorNode("node1")
			ts.AddDefaultWallet(node)
			ts.Run(true)
			tx1 := ts.DefaultWallet().CreateAccountFromInput(
				"tx1",
				"Genesis:0",
				ts.DefaultWallet(),
				mock.WithBlockIssuerFeature(iotago.BlockIssuerKeys{tpkg.RandBlockIssuerKey()}, iotago.MaxSlotIndex),
				mock.WithStakingFeature(10000, 421, 0, 10),
				mock.WithAccountMana(mock.MaxBlockManaCost(ts.DefaultWallet().Node.Protocol.CommittedAPI().ProtocolParameters())),
			)
			// default block issuer issues a block containing the transaction in slot 1.
			genesisCommitment := iotago.NewEmptyCommitment(ts.API)
			genesisCommitment.ReferenceManaCost = ts.API.ProtocolParameters().CongestionControlParameters().MinReferenceManaCost
			block1 := ts.IssueBasicBlockWithOptions("block1", ts.DefaultWallet(), tx1, mock.WithSlotCommitment(genesisCommitment))
			block = block1.ProtocolBlock()
			modelBlock := lo.PanicOnErr(model.BlockFromBlock(block))
			node.Protocol.Events.Engine.Scheduler.BlockScheduled.Hook(func(block *blocks.Block) {
				blockchan <- block
			})
			b.StartTimer()
			// time from issuance of the block to when it is scheduled
			node.Protocol.IssueBlock(modelBlock)
			<-blockchan
			b.StopTimer()
			ts.Shutdown()
		}
	}
	// get the ns/op of processing the block
	nsPerBlock := float64(testing.Benchmark(fn).NsPerOp())
	fmt.Printf("One input one account with staking output and remainder: %f ns/op\n", nsPerBlock)
	// get the regressors
	regressors := GetBlockWorkScoreRegressors(block)
	printRegressors(regressors)

	return nsPerBlock, regressors
}

func oneInAccRemOut(t *testing.T) (float64, []float64) {
	blockchan := make(chan *blocks.Block, 1)
	var block *iotago.Block

	// basic block with one input, one account output with staking and a remainder
	fn := func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			b.StopTimer()
			ts := NewTestSuite(t)
			node := ts.AddValidatorNode("node1")
			ts.AddDefaultWallet(node)
			ts.Run(true)
			tx1 := ts.DefaultWallet().CreateAccountFromInput(
				"tx1",
				"Genesis:0",
				ts.DefaultWallet(),
				mock.WithBlockIssuerFeature(iotago.BlockIssuerKeys{tpkg.RandBlockIssuerKey()}, iotago.MaxSlotIndex),
				mock.WithAccountAmount(100000),
				mock.WithAccountMana(mock.MaxBlockManaCost(ts.DefaultWallet().Node.Protocol.CommittedAPI().ProtocolParameters())),
			)
			// default block issuer issues a block containing the transaction in slot 1.
			genesisCommitment := iotago.NewEmptyCommitment(ts.API)
			genesisCommitment.ReferenceManaCost = ts.API.ProtocolParameters().CongestionControlParameters().MinReferenceManaCost
			block1 := ts.IssueBasicBlockWithOptions("block1", ts.DefaultWallet(), tx1, mock.WithSlotCommitment(genesisCommitment))
			block = block1.ProtocolBlock()
			modelBlock := lo.PanicOnErr(model.BlockFromBlock(block))
			node.Protocol.Events.Engine.Scheduler.BlockScheduled.Hook(func(block *blocks.Block) {
				blockchan <- block
			})
			b.StartTimer()
			// time from issuance of the block to when it is scheduled
			node.Protocol.IssueBlock(modelBlock)
			<-blockchan
			b.StopTimer()
			ts.Shutdown()
		}
	}
	// get the ns/op of processing the block
	nsPerBlock := float64(testing.Benchmark(fn).NsPerOp())
	fmt.Printf("One input one account output and remainder: %f ns/op\n", nsPerBlock)
	// get the regressors
	regressors := GetBlockWorkScoreRegressors(block)
	printRegressors(regressors)

	return nsPerBlock, regressors
}

func oneInAccStakingRemOut(t *testing.T) (float64, []float64) {
	blockchan := make(chan *blocks.Block, 1)
	var block *iotago.Block

	// basic block with one input, one account output with staking and a remainder
	fn := func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			b.StopTimer()
			ts := NewTestSuite(t)
			node := ts.AddValidatorNode("node1")
			ts.AddDefaultWallet(node)
			ts.Run(true)
			tx1 := ts.DefaultWallet().CreateAccountFromInput(
				"tx1",
				"Genesis:0",
				ts.DefaultWallet(),
				mock.WithBlockIssuerFeature(iotago.BlockIssuerKeys{tpkg.RandBlockIssuerKey()}, iotago.MaxSlotIndex),
				mock.WithStakingFeature(10000, 421, 0, 10),
				mock.WithAccountAmount(100000),
				mock.WithAccountMana(mock.MaxBlockManaCost(ts.DefaultWallet().Node.Protocol.CommittedAPI().ProtocolParameters())),
			)
			// default block issuer issues a block containing the transaction in slot 1.
			genesisCommitment := iotago.NewEmptyCommitment(ts.API)
			genesisCommitment.ReferenceManaCost = ts.API.ProtocolParameters().CongestionControlParameters().MinReferenceManaCost
			block1 := ts.IssueBasicBlockWithOptions("block1", ts.DefaultWallet(), tx1, mock.WithSlotCommitment(genesisCommitment))
			block = block1.ProtocolBlock()
			modelBlock := lo.PanicOnErr(model.BlockFromBlock(block))
			node.Protocol.Events.Engine.Scheduler.BlockScheduled.Hook(func(block *blocks.Block) {
				blockchan <- block
			})
			b.StartTimer()
			// time from issuance of the block to when it is scheduled
			node.Protocol.IssueBlock(modelBlock)
			<-blockchan
			b.StopTimer()
			ts.Shutdown()
		}
	}
	// get the ns/op of processing the block
	nsPerBlock := float64(testing.Benchmark(fn).NsPerOp())
	fmt.Printf("One input one account with staking output and remainder: %f ns/op\n", nsPerBlock)
	// get the regressors
	regressors := GetBlockWorkScoreRegressors(block)
	printRegressors(regressors)

	return nsPerBlock, regressors
}

func accInAccOut(t *testing.T) (float64, []float64) {
	blockchan := make(chan *blocks.Block, 1)
	var block *iotago.Block

	// basic block with one input, one account output with staking and a remainder
	fn := func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			b.StopTimer()
			ts := NewTestSuite(t)
			node := ts.AddValidatorNode("node1")
			ts.AddDefaultWallet(node)
			ts.Run(true)
			tx1 := ts.DefaultWallet().TransitionAccount(
				"tx1",
				"Genesis:2",
				mock.WithBlockIssuerFeature(iotago.BlockIssuerKeys{tpkg.RandBlockIssuerKey()}, iotago.MaxSlotIndex),
			)
			// default block issuer issues a block containing the transaction in slot 1.
			genesisCommitment := iotago.NewEmptyCommitment(ts.API)
			genesisCommitment.ReferenceManaCost = ts.API.ProtocolParameters().CongestionControlParameters().MinReferenceManaCost
			block1 := ts.IssueBasicBlockWithOptions("block1", ts.DefaultWallet(), tx1, mock.WithSlotCommitment(genesisCommitment))
			block = block1.ProtocolBlock()
			modelBlock := lo.PanicOnErr(model.BlockFromBlock(block))
			node.Protocol.Events.Engine.Scheduler.BlockScheduled.Hook(func(block *blocks.Block) {
				blockchan <- block
			})
			b.StartTimer()
			// time from issuance of the block to when it is scheduled
			node.Protocol.IssueBlock(modelBlock)
			<-blockchan
			b.StopTimer()
			ts.Shutdown()
		}
	}
	// get the ns/op of processing the block
	nsPerBlock := float64(testing.Benchmark(fn).NsPerOp())
	fmt.Printf("One account input one account output: %f ns/op\n", nsPerBlock)
	// get the regressors
	regressors := GetBlockWorkScoreRegressors(block)
	printRegressors(regressors)

	return nsPerBlock, regressors
}

func accInAccStakingOut(t *testing.T) (float64, []float64) {
	blockchan := make(chan *blocks.Block, 1)
	var block *iotago.Block

	// basic block with one input, one account output with staking and a remainder
	fn := func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			b.StopTimer()
			ts := NewTestSuite(t)
			node := ts.AddValidatorNode("node1")
			ts.AddDefaultWallet(node)
			ts.Run(true)
			tx1 := ts.DefaultWallet().TransitionAccount(
				"tx1",
				"Genesis:2",
				mock.WithBlockIssuerFeature(iotago.BlockIssuerKeys{tpkg.RandBlockIssuerKey()}, iotago.MaxSlotIndex),
				mock.WithStakingFeature(10000, 421, 0, 10),
			)
			// default block issuer issues a block containing the transaction in slot 1.
			genesisCommitment := iotago.NewEmptyCommitment(ts.API)
			genesisCommitment.ReferenceManaCost = ts.API.ProtocolParameters().CongestionControlParameters().MinReferenceManaCost
			block1 := ts.IssueBasicBlockWithOptions("block1", ts.DefaultWallet(), tx1, mock.WithSlotCommitment(genesisCommitment))
			block = block1.ProtocolBlock()
			modelBlock := lo.PanicOnErr(model.BlockFromBlock(block))
			node.Protocol.Events.Engine.Scheduler.BlockScheduled.Hook(func(block *blocks.Block) {
				blockchan <- block
			})
			b.StartTimer()
			// time from issuance of the block to when it is scheduled
			node.Protocol.IssueBlock(modelBlock)
			<-blockchan
			b.StopTimer()
			ts.Shutdown()
		}
	}
	// get the ns/op of processing the block
	nsPerBlock := float64(testing.Benchmark(fn).NsPerOp())
	fmt.Printf("One account input one account output with staking: %f ns/op\n", nsPerBlock)
	// get the regressors
	regressors := GetBlockWorkScoreRegressors(block)
	printRegressors(regressors)

	return nsPerBlock, regressors
}

func basicInNativeOut(t *testing.T, nNative int) (float64, []float64) {
	if nNative > iotago.MaxOutputsCount-1 {
		panic("Can only create MaxOutputsCount - 1 native token outputs because we need an account output as well")
	}
	blockchan := make(chan *blocks.Block, 1)
	var block *iotago.Block
	fn := func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			b.StopTimer()
			ts := NewTestSuite(t)
			ts.AddDefaultWallet(ts.AddValidatorNode("node1"))
			ts.Run(true)
			var addressIndexes []uint32
			for i := 0; i < nNative-1; i++ {
				addressIndexes = append(addressIndexes, uint32(i))
			}
			tx1 := ts.DefaultWallet().CreateFoundryAndNativeTokensFromInput(
				"tx1",
				"Genesis:0",
				"Genesis:2",
				addressIndexes...,
			)
			genesisCommitment := iotago.NewEmptyCommitment(ts.API)
			genesisCommitment.ReferenceManaCost = ts.API.ProtocolParameters().CongestionControlParameters().MinReferenceManaCost
			// issue a block with the transaction
			block1 := ts.IssueBasicBlockWithOptions("block1", ts.DefaultWallet(), tx1, mock.WithSlotCommitment(genesisCommitment))
			// get the protocol block
			block = block1.ProtocolBlock()
			// get the model block
			modelBlock := lo.PanicOnErr(model.BlockFromBlock(block))
			// hook the block scheduled event
			ts.DefaultWallet().Node.Protocol.Events.Engine.Scheduler.BlockScheduled.Hook(func(block *blocks.Block) {
				blockchan <- block
			})
			// start the timer
			b.StartTimer()
			// time from issuance of the block to when it is scheduled
			ts.DefaultWallet().Node.Protocol.IssueBlock(modelBlock)
			<-blockchan
			// stop the timer
			b.StopTimer()
			// shutdown the test suite
			ts.Shutdown()
		}
	}
	// get the ns/op of processing the block
	nsPerBlock := float64(testing.Benchmark(fn).NsPerOp())

	fmt.Printf("One input max native token outputs: %f ns/op\n", nsPerBlock)

	// get the regressors
	regressors := GetBlockWorkScoreRegressors(block)
	printRegressors(regressors)
	return nsPerBlock, regressors
}

func nativeInNativeOut(t *testing.T) (float64, []float64) {
	blockchan := make(chan *blocks.Block, 1)
	var block *iotago.Block

	// basic block with one input and one output
	fn := func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			b.StopTimer()
			ts := NewTestSuite(t)
			node := ts.AddValidatorNode("node1")
			ts.AddDefaultWallet(node)
			ts.Run(true)
			node.Protocol.Events.Engine.Scheduler.BlockScheduled.Hook(func(block *blocks.Block) {
				blockchan <- block
			})
			tx1 := ts.DefaultWallet().CreateFoundryAndNativeTokensFromInput(
				"tx1",
				"Genesis:0",
				"Genesis:2",
			)
			// default block issuer issues a block containing the transaction in slot 1.
			genesisCommitment := iotago.NewEmptyCommitment(ts.API)
			genesisCommitment.ReferenceManaCost = ts.API.ProtocolParameters().CongestionControlParameters().MinReferenceManaCost
			block1 := ts.IssueBasicBlockWithOptions("block1", ts.DefaultWallet(), tx1, mock.WithSlotCommitment(genesisCommitment))
			block = block1.ProtocolBlock()
			modelBlock := lo.PanicOnErr(model.BlockFromBlock(block))
			node.Protocol.Events.Engine.Scheduler.BlockScheduled.Hook(func(block *blocks.Block) {
				blockchan <- block
			})
			node.Protocol.IssueBlock(modelBlock)
			<-blockchan

			tx2 := ts.DefaultWallet().TransitionFoundry(
				"tx2",
				"tx1:0",
				"tx1:1",
			)
			genesisCommitment.ReferenceManaCost = ts.API.ProtocolParameters().CongestionControlParameters().MinReferenceManaCost
			block2 := ts.IssueBasicBlockWithOptions("block2", ts.DefaultWallet(), tx2, mock.WithSlotCommitment(genesisCommitment))
			block = block2.ProtocolBlock()
			modelBlock = lo.PanicOnErr(model.BlockFromBlock(block))
			b.StartTimer()
			// time from issuance of the block to when it is scheduled
			node.Protocol.IssueBlock(modelBlock)
			<-blockchan
			b.StopTimer()
			ts.Shutdown()
		}
	}
	// get the ns/op of processing the block
	nsPerBlock := float64(testing.Benchmark(fn).NsPerOp())
	fmt.Printf("One native token in one native token output: %f ns/op\n", nsPerBlock)
	// get the regressors
	regressors := GetBlockWorkScoreRegressors(block)
	printRegressors(regressors)

	return nsPerBlock, regressors
}

func allotments(t *testing.T, numAllotments int) (float64, []float64) {
	blockchan := make(chan *blocks.Block, 1)
	var block *iotago.Block

	// basic block with one input and one output
	fn := func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			b.StopTimer()
			ts := NewTestSuite(t)
			// create genesis accounts to allot to.
			for i := 0; i < numAllotments; i++ {
				ts.AddGenesisAccount(snapshotcreator.AccountDetails{
					Address:              nil,
					Amount:               mock.MinIssuerAccountAmount(ts.API.ProtocolParameters()) * 10,
					Mana:                 0,
					IssuerKey:            tpkg.RandBlockIssuerKey(),
					ExpirySlot:           iotago.MaxSlotIndex,
					BlockIssuanceCredits: iotago.BlockIssuanceCredits(123),
				})
			}
			node := ts.AddValidatorNode("node1")
			ts.AddDefaultWallet(node)
			ts.Run(true)
			var accountIDs []iotago.AccountID
			for i := 0; i < numAllotments; i++ {
				accountOutput := ts.AccountOutput(fmt.Sprintf("Genesis:%d", i+1)).Output().(*iotago.AccountOutput)
				accountIDs = append(accountIDs, accountOutput.AccountID)
			}
			tx1 := ts.DefaultWallet().AllotManaFromInput(
				"tx1",
				"Genesis:0",
				accountIDs...,
			)
			// default block issuer issues a block containing the transaction in slot 1.
			genesisCommitment := iotago.NewEmptyCommitment(ts.API)
			genesisCommitment.ReferenceManaCost = ts.API.ProtocolParameters().CongestionControlParameters().MinReferenceManaCost
			block1 := ts.IssueBasicBlockWithOptions("block1", ts.DefaultWallet(), tx1, mock.WithSlotCommitment(genesisCommitment))
			block = block1.ProtocolBlock()
			modelBlock := lo.PanicOnErr(model.BlockFromBlock(block))
			node.Protocol.Events.Engine.Scheduler.BlockScheduled.Hook(func(block *blocks.Block) {
				blockchan <- block
			})
			b.StartTimer()
			// time from issuance of the block to when it is scheduled
			node.Protocol.IssueBlock(modelBlock)
			<-blockchan
			b.StopTimer()
			ts.Shutdown()
		}
	}
	// get the ns/op of processing the block
	nsPerBlock := float64(testing.Benchmark(fn).NsPerOp())
	fmt.Printf("One input: %f ns/op\n", nsPerBlock)
	// get the regressors
	regressors := GetBlockWorkScoreRegressors(block)
	printRegressors(regressors)

	return nsPerBlock, regressors
}

func printRegressors(regressors []float64) {
	fmt.Printf("Input: %f\n", regressors[0])
	fmt.Printf("ContextInput: %f\n", regressors[1])
	fmt.Printf("Output: %f\n", regressors[2])
	fmt.Printf("NativeToken: %f\n", regressors[3])
	fmt.Printf("Staking: %f\n", regressors[4])
	fmt.Printf("BlockIssuer: %f\n", regressors[5])
	fmt.Printf("Allotment: %f\n", regressors[6])
	fmt.Printf("SignatureEd25519: %f\n", regressors[7])
}

func printCoefficients(coefficients []float64) {
	minCoeff := math.Abs(coefficients[0])
	for _, coeff := range coefficients {
		if math.Abs(coeff) < minCoeff {
			minCoeff = coeff
		}
	}
	normalisedCoeffs := make([]float64, len(coefficients))
	for i, coeff := range coefficients {
		normalisedCoeffs[i] = coeff / minCoeff
	}

	fmt.Println("Calculated coefficients from regression:")
	fmt.Printf("Block: %f\n", normalisedCoeffs[0])
	fmt.Printf("Input: %f\n", normalisedCoeffs[1])
	fmt.Printf("ContextInput: %f\n", normalisedCoeffs[2])
	fmt.Printf("Output: %f\n", normalisedCoeffs[3])
	fmt.Printf("NativeToken: %f\n", normalisedCoeffs[4])
	fmt.Printf("Staking: %f\n", normalisedCoeffs[5])
	fmt.Printf("BlockIssuer: %f\n", normalisedCoeffs[6])
	fmt.Printf("Allotment: %f\n", normalisedCoeffs[7])
	fmt.Printf("SignatureEd25519: %f\n", normalisedCoeffs[8])
}

func GetBlockWorkScoreRegressors(block *iotago.Block) []float64 {
	regressors := make([]float64, 8)

	basicBlockBody, isBasic := block.Body.(*iotago.BasicBlockBody)
	if !isBasic {
		panic("block body is not a basic block body")
	}
	signedTx, isSignedTx := basicBlockBody.Payload.(*iotago.SignedTransaction)
	if !isSignedTx {
		panic("block payload is not a signed transaction")
	}
	// add one to the Input regressor for each input
	regressors[0] += float64(len(signedTx.Transaction.TransactionEssence.Inputs))
	// add one to the ContextInput regressor for each context input
	regressors[1] += float64(len(signedTx.Transaction.TransactionEssence.ContextInputs))
	for _, output := range signedTx.Transaction.Outputs {
		// add one to the Output regressor for each output
		regressors[2] += 1
		for _, feature := range output.FeatureSet() {
			switch feature.Type() {
			case iotago.FeatureNativeToken:
				// add one to the NativeToken regressor for each output with the native token feature
				regressors[3] += 1
			case iotago.FeatureStaking:
				// add one to the Staking regressor for each output with the staking feature
				regressors[4] += 1
			case iotago.FeatureBlockIssuer:
				// add one to the BlockIssuer regressor for each output with the block issuer feature
				regressors[5] += 1
			}
		}
	}
	// add one to Allotments regressor for each allotment
	regressors[6] += float64(len(signedTx.Transaction.TransactionEssence.Allotments))
	for _, unlock := range signedTx.Unlocks {
		if unlock.Type() == iotago.UnlockSignature {
			// add one to the SignatureEd25519 regressor for each unlock block
			regressors[7] += 1
		}
	}

	return regressors
}
