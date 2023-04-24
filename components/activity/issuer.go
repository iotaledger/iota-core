package activity

import (
	"github.com/iotaledger/hive.go/lo"
	iotago "github.com/iotaledger/iota.go/v4"
)

func issuerID() iotago.AccountID {
	issuerKey := lo.PanicOnErr(deps.Peer.Database().LocalPrivateKey())
	pubKey := issuerKey.Public()

	return iotago.AccountID(iotago.Ed25519AddressFromPubKey(pubKey[:]))
}

func issueActivityBlock() {
	if !ParamsActivity.IgnoreBootstrapped && !deps.Protocol.MainEngineInstance().IsBootstrapped() {
		Component.LogDebug("Not issuing activity block because node is not bootstrapped yet.")
		return
	}

	block, err := deps.Protocol.BlockIssuer.IssuePayload(&iotago.TaggedData{
		Tag: []byte("ACTIVITY"),
	})
	if err != nil {
		Component.LogWarnf("error building block: %s", err.Error())
		return
	}

	Component.LogInfof("Issued activity block: %s - commitment %s %d - latest finalized slot %d", block.ID(), block.Block().SlotCommitment.MustID(), block.Block().SlotCommitment.Index, block.Block().LatestFinalizedSlot)
}
