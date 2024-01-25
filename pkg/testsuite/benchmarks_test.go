package testsuite

import (
	"fmt"
	"testing"

	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/tpkg"
	"github.com/sajari/regression"
)

// Test_Regression runs benchmarks for many block types and find the best fit regression model.
func Test_Regression(t *testing.T) {
	r := new(regression.Regression)
	r.SetObserved("ns/op")
	r.SetVar(0, "DataByte")
	r.SetVar(1, "Block")
	r.SetVar(2, "Input")
	r.SetVar(3, "ContextInput")
	r.SetVar(4, "Output")
	r.SetVar(5, "NativeToken")
	r.SetVar(6, "Staking")
	r.SetVar(7, "BlockIssuer")
	r.SetVar(8, "Allotment")
	r.SetVar(9, "SignatureEd25519")

	r.Train(
		regression.DataPoint(benchmarkOneIO(t)),
		regression.DataPoint(benchmarkOneAccountOutputStaking(t)),
		regression.DataPoint(benchmarkOneIMaxO(t)),
	)
	r.Run()
}

func benchmarkOneIO(t *testing.T) (float64, []float64) {
	blockchan := make(chan *blocks.Block, 1)
	var block *iotago.Block

	// basic block with one input and one output
	fn := func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			ts := NewTestSuite(t)
			node := ts.AddValidatorNode("node1")
			ts.AddDefaultWallet(node)
			ts.Run(true)
			tx1 := ts.DefaultWallet().CreateBasicOutputsEquallyFromInput(
				"tx1",
				1,
				"Genesis:0",
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

func benchmarkOneIMaxO(t *testing.T) (float64, []float64) {
	blockchan := make(chan *blocks.Block, 1)
	var block *iotago.Block

	// basic block with one input and one output
	fn := func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			ts := NewTestSuite(t)
			node := ts.AddValidatorNode("node1")
			ts.AddDefaultWallet(node)
			ts.Run(true)
			tx1 := ts.DefaultWallet().CreateBasicOutputsEquallyFromInput(
				"tx1",
				iotago.MaxOutputsCount,
				"Genesis:0",
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
		}
	}
	// get the ns/op of processing the block
	nsPerBlock := float64(testing.Benchmark(fn).NsPerOp())
	fmt.Printf("One input max outputs: %f ns/op\n", nsPerBlock)
	// get the regressors
	regressors := GetBlockWorkScoreRegressors(block)
	printRegressors(regressors)

	return nsPerBlock, regressors
}

func benchmarkOneAccountOutputStaking(t *testing.T) (float64, []float64) {
	blockchan := make(chan *blocks.Block, 1)
	var block *iotago.Block

	// basic block with one input, one account output with staking and a remainder
	fn := func(b *testing.B) {
		for i := 0; i < b.N; i++ {
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
		}
	}
	// get the ns/op of processing the block
	nsPerBlock := float64(testing.Benchmark(fn).NsPerOp())
	fmt.Printf("One input one account with staking output: %f ns/op\n", nsPerBlock)
	// get the regressors
	regressors := GetBlockWorkScoreRegressors(block)
	printRegressors(regressors)

	return nsPerBlock, regressors
}

func printRegressors(regressors []float64) {
	fmt.Printf("DataByte: %f\n", regressors[0])
	fmt.Printf("Block: %f\n", regressors[1])
	fmt.Printf("Input: %f\n", regressors[2])
	fmt.Printf("ContextInput: %f\n", regressors[3])
	fmt.Printf("Output: %f\n", regressors[4])
	fmt.Printf("NativeToken: %f\n", regressors[5])
	fmt.Printf("Staking: %f\n", regressors[6])
	fmt.Printf("BlockIssuer: %f\n", regressors[7])
	fmt.Printf("Allotment: %f\n", regressors[8])
	fmt.Printf("SignatureEd25519: %f\n", regressors[9])
}

func GetBlockWorkScoreRegressors(block *iotago.Block) []float64 {
	regressors := make([]float64, 10)

	basicBlockBody, isBasic := block.Body.(*iotago.BasicBlockBody)
	if !isBasic {
		panic("block body is not a basic block body")
	}
	signedTx, isSignedTx := basicBlockBody.Payload.(*iotago.SignedTransaction)
	if !isSignedTx {
		panic("block payload is not a signed transaction")
	}
	// get the number of bytes of the payload as the DataByte regressor
	regressors[0] = float64(signedTx.Size())
	// this is a block, so the Block regressor is 1
	regressors[1] = 1
	// add one to the Input regressor for each input
	regressors[2] += float64(len(signedTx.Transaction.TransactionEssence.Inputs))
	// add one to the ContextInput regressor for each context input
	regressors[3] += float64(len(signedTx.Transaction.TransactionEssence.ContextInputs))
	for _, output := range signedTx.Transaction.Outputs {
		// add one to the Output regressor for each output
		regressors[4] += 1
		for _, feature := range output.FeatureSet() {
			switch feature.Type() {
			case iotago.FeatureNativeToken:
				// add one to the NativeToken regressor for each output with the native token feature
				regressors[5] += 1
			case iotago.FeatureStaking:
				// add one to the Staking regressor for each output with the staking feature
				regressors[6] += 1
			case iotago.FeatureBlockIssuer:
				// add one to the BlockIssuer regressor for each output with the block issuer feature
				regressors[7] += 1
			}
		}
	}
	// add one to Allotments regressor for each allotment
	regressors[8] += float64(len(signedTx.Transaction.TransactionEssence.Allotments))
	// all blocks have an Ed25519 signature, so the SignatureEd25519 regressor is at least 1
	regressors[9] = 1
	for _, unlock := range signedTx.Unlocks {
		if unlock.Type() == iotago.UnlockSignature {
			// add one to the SignatureEd25519 regressor for each unlock block
			regressors[9] += 1
		}
	}

	return regressors
}
