package testsuite

import (
	"fmt"
	"testing"

	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
)

// BenchmarkProcessOneIOTxBlock benchmarks the processing of a block containing a transaction with one input and one output.
func Test_BenchmarkProcessOneIOTxBlock(t *testing.T) {
	blockchan := make(chan *blocks.Block, 1)
	var block *iotago.Block
	var y []int64                      // y is the dependent variable
	var X []iotago.WorkScoreParameters // X is the independent variable
	var fn func(b *testing.B)

	// basic block with one input and one output
	fn = func(b *testing.B) {
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
	nsPerBlock := testing.Benchmark(fn).NsPerOp()
	fmt.Printf("One input One output: %d ns/op\n", nsPerBlock)
	y = append(y, nsPerBlock)
	// get the regressors
	regressors := GetBlockWorkScoreRegressors(block)
	printRegressors(regressors)
	X = append(X, regressors)

	// basic block with one input and max output count
	fn = func(b *testing.B) {
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
	nsPerBlock = testing.Benchmark(fn).NsPerOp()
	fmt.Printf("One input max outputs: %d ns/op\n", nsPerBlock)
	y = append(y, nsPerBlock)
	// get the regressors
	regressors = GetBlockWorkScoreRegressors(block)
	printRegressors(regressors)
	X = append(X, regressors)
}

func printRegressors(regressors iotago.WorkScoreParameters) {
	fmt.Printf("Block: %d\n", regressors.Block)
	fmt.Printf("SignatureEd25519: %d\n", regressors.SignatureEd25519)
	fmt.Printf("DataByte: %d\n", regressors.DataByte)
	fmt.Printf("Allotment: %d\n", regressors.Allotment)
	fmt.Printf("Input: %d\n", regressors.Input)
	fmt.Printf("ContextInput: %d\n", regressors.ContextInput)
	fmt.Printf("Output: %d\n", regressors.Output)
	fmt.Printf("NativeToken: %d\n", regressors.NativeToken)
	fmt.Printf("Staking: %d\n", regressors.Staking)
	fmt.Printf("BlockIssuer: %d\n", regressors.BlockIssuer)
}

func GetBlockWorkScoreRegressors(block *iotago.Block) iotago.WorkScoreParameters {
	var regressors iotago.WorkScoreParameters

	// this is a block, so the Block regressor is 1
	regressors.Block = 1
	// all blocks have an Ed25519 signature, so the SignatureEd25519 regressor is at least 1
	regressors.SignatureEd25519 = 1
	basicBlockBody, isBasic := block.Body.(*iotago.BasicBlockBody)
	if !isBasic {
		panic("block body is not a basic block body")
	}
	signedTx, isSignedTx := basicBlockBody.Payload.(*iotago.SignedTransaction)
	if !isSignedTx {
		panic("block payload is not a signed transaction")
	}
	// get the number of bytes of the payload as the DataByte regressor
	regressors.DataByte = iotago.WorkScore(signedTx.Size())
	// add one to Allotments regressor for each allotment
	regressors.Allotment += iotago.WorkScore(len(signedTx.Transaction.TransactionEssence.Allotments))
	// add one to the Input regressor for each input
	regressors.Input += iotago.WorkScore(len(signedTx.Transaction.TransactionEssence.Inputs))
	// add one to the ContextInput regressor for each context input
	regressors.ContextInput += iotago.WorkScore(len(signedTx.Transaction.TransactionEssence.ContextInputs))
	for _, output := range signedTx.Transaction.Outputs {
		// add one to the Output regressor for each output
		regressors.Output += 1
		for _, feature := range output.FeatureSet() {
			switch feature.Type() {
			case iotago.FeatureNativeToken:
				// add one to the NativeToken regressor for each output with the native token feature
				regressors.NativeToken += 1
			case iotago.FeatureStaking:
				// add one to the Staking regressor for each output with the staking feature
				regressors.Staking += 1
			case iotago.FeatureBlockIssuer:
				// add one to the BlockIssuer regressor for each output with the block issuer feature
				regressors.BlockIssuer += 1
			}
		}
	}
	for _, unlock := range signedTx.Unlocks {
		if unlock.Type() == iotago.UnlockSignature {
			// add one to the SignatureEd25519 regressor for each unlock block
			regressors.SignatureEd25519 += 1
		}
	}

	return regressors
}
