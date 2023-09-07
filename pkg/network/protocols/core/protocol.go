package core

import (
	"encoding/json"

	"google.golang.org/protobuf/proto"

	"github.com/iotaledger/hive.go/ds/bytesfilter"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/ds/types"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/hive.go/serializer/v2/serix"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/network"
	nwmodels "github.com/iotaledger/iota-core/pkg/network/protocols/core/models"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
	"github.com/iotaledger/iota.go/v4/merklehasher"
)

const (
	protocolID = "iota-core/0.0.1"
)

type Protocol struct {
	Events *Events

	apiProvider api.Provider

	network                   network.Endpoint
	workerPool                *workerpool.WorkerPool
	duplicateBlockBytesFilter *bytesfilter.BytesFilter

	requestedBlockHashes      *shrinkingmap.ShrinkingMap[types.Identifier, types.Empty]
	requestedBlockHashesMutex syncutils.Mutex
}

func NewProtocol(network network.Endpoint, workerPool *workerpool.WorkerPool, apiProvider api.Provider, opts ...options.Option[Protocol]) (protocol *Protocol) {
	return options.Apply(&Protocol{
		Events: NewEvents(),

		network:                   network,
		workerPool:                workerPool,
		apiProvider:               apiProvider,
		duplicateBlockBytesFilter: bytesfilter.New(10000),
		requestedBlockHashes:      shrinkingmap.New[types.Identifier, types.Empty](shrinkingmap.WithShrinkingThresholdCount(1000)),
	}, opts, func(p *Protocol) {
		network.RegisterProtocol(protocolID, newPacket, p.handlePacket)
	})
}

func (p *Protocol) SendBlock(block *model.Block, to ...network.PeerID) {
	p.network.Send(&nwmodels.Packet{Body: &nwmodels.Packet_Block{Block: &nwmodels.Block{
		Bytes: block.Data(),
	}}}, protocolID, to...)
}

func (p *Protocol) RequestBlock(id iotago.BlockID, to ...network.PeerID) {
	p.requestedBlockHashesMutex.Lock()
	p.requestedBlockHashes.Set(types.Identifier(id.Identifier()), types.Void)
	p.requestedBlockHashesMutex.Unlock()

	p.network.Send(&nwmodels.Packet{Body: &nwmodels.Packet_BlockRequest{BlockRequest: &nwmodels.BlockRequest{
		BlockId: id[:],
	}}}, protocolID, to...)
}

func (p *Protocol) SendSlotCommitment(cm *model.Commitment, to ...network.PeerID) {
	p.network.Send(&nwmodels.Packet{Body: &nwmodels.Packet_SlotCommitment{SlotCommitment: &nwmodels.SlotCommitment{
		Bytes: cm.Data(),
	}}}, protocolID, to...)
}

func (p *Protocol) SendAttestations(cm *model.Commitment, attestations []*iotago.Attestation, merkleProof *merklehasher.Proof[iotago.Identifier], to ...network.PeerID) {
	var iotagoAPI iotago.API
	if len(attestations) > 0 {
		// TODO: there are multiple attestations potentially spanning multiple epochs/versions, we need to use the correct API for each one
		iotagoAPI = lo.PanicOnErr(p.apiProvider.APIForVersion(attestations[0].ProtocolVersion))
	} else {
		iotagoAPI = p.apiProvider.APIForSlot(cm.Index()) // we need an api to serialize empty slices as well
	}
	p.network.Send(&nwmodels.Packet{Body: &nwmodels.Packet_Attestations{Attestations: &nwmodels.Attestations{
		Commitment:   cm.Data(),
		Attestations: lo.PanicOnErr(iotagoAPI.Encode(attestations)),
		MerkleProof:  lo.PanicOnErr(json.Marshal(merkleProof)),
	}}}, protocolID, to...)
}

func (p *Protocol) RequestSlotCommitment(id iotago.CommitmentID, to ...network.PeerID) {
	p.network.Send(&nwmodels.Packet{Body: &nwmodels.Packet_SlotCommitmentRequest{SlotCommitmentRequest: &nwmodels.SlotCommitmentRequest{
		CommitmentId: id[:],
	}}}, protocolID, to...)
}

func (p *Protocol) RequestAttestations(id iotago.CommitmentID, to ...network.PeerID) {
	p.network.Send(&nwmodels.Packet{Body: &nwmodels.Packet_AttestationsRequest{AttestationsRequest: &nwmodels.AttestationsRequest{
		CommitmentId: lo.PanicOnErr(id.Bytes()),
	}}}, protocolID, to...)
}

func (p *Protocol) OnBlockReceived(callback func(block *model.Block, src network.PeerID)) (unsubscribe func()) {
	return p.Events.BlockReceived.Hook(callback).Unhook
}

func (p *Protocol) OnBlockRequestReceived(callback func(blockID iotago.BlockID, src network.PeerID)) (unsubscribe func()) {
	return p.Events.BlockRequestReceived.Hook(callback).Unhook
}

func (p *Protocol) OnSlotCommitmentReceived(callback func(commitment *model.Commitment, src network.PeerID)) (unsubscribe func()) {
	return p.Events.SlotCommitmentReceived.Hook(callback).Unhook
}

func (p *Protocol) OnSlotCommitmentRequestReceived(callback func(commitmentID iotago.CommitmentID, src network.PeerID)) (unsubscribe func()) {
	return p.Events.SlotCommitmentRequestReceived.Hook(callback).Unhook
}

func (p *Protocol) OnAttestationsReceived(callback func(*model.Commitment, []*iotago.Attestation, *merklehasher.Proof[iotago.Identifier], network.PeerID)) (unsubscribe func()) {
	return p.Events.AttestationsReceived.Hook(callback).Unhook
}

func (p *Protocol) OnAttestationsRequestReceived(callback func(commitmentID iotago.CommitmentID, src network.PeerID)) (unsubscribe func()) {
	return p.Events.AttestationsRequestReceived.Hook(callback).Unhook
}

func (p *Protocol) OnWarpSyncResponseReceived(callback func(commitmentID iotago.CommitmentID, blockIDs iotago.BlockIDs, proof *merklehasher.Proof[iotago.Identifier], src network.PeerID)) (unsubscribe func()) {
	return p.Events.WarpSyncResponseReceived.Hook(callback).Unhook
}

func (p *Protocol) OnWarpSyncRequestReceived(callback func(commitmentID iotago.CommitmentID, src network.PeerID)) (unsubscribe func()) {
	return p.Events.WarpSyncRequestReceived.Hook(callback).Unhook
}

func (p *Protocol) OnError(callback func(err error, src network.PeerID)) (unsubscribe func()) {
	return p.Events.Error.Hook(callback).Unhook
}

func (p *Protocol) Shutdown() {
	p.network.UnregisterProtocol(protocolID)

	p.workerPool.Shutdown()
	p.workerPool.ShutdownComplete.Wait()
}

func (p *Protocol) handlePacket(nbr network.PeerID, packet proto.Message) (err error) {
	switch packetBody := packet.(*nwmodels.Packet).GetBody().(type) {
	case *nwmodels.Packet_Block:
		p.workerPool.Submit(func() { p.onBlock(packetBody.Block.GetBytes(), nbr) })
	case *nwmodels.Packet_BlockRequest:
		p.workerPool.Submit(func() { p.onBlockRequest(packetBody.BlockRequest.GetBlockId(), nbr) })
	case *nwmodels.Packet_SlotCommitment:
		p.workerPool.Submit(func() { p.onSlotCommitment(packetBody.SlotCommitment.GetBytes(), nbr) })
	case *nwmodels.Packet_SlotCommitmentRequest:
		p.workerPool.Submit(func() { p.onSlotCommitmentRequest(packetBody.SlotCommitmentRequest.GetCommitmentId(), nbr) })
	case *nwmodels.Packet_Attestations:
		p.workerPool.Submit(func() {
			p.onAttestations(packetBody.Attestations.GetCommitment(), packetBody.Attestations.GetAttestations(), packetBody.Attestations.GetMerkleProof(), nbr)
		})
	case *nwmodels.Packet_AttestationsRequest:
		p.workerPool.Submit(func() {
			p.onAttestationsRequest(packetBody.AttestationsRequest.GetCommitmentId(), nbr)
		})
	case *nwmodels.Packet_WarpSyncRequest:
		p.handleWarpSyncRequest(packetBody.WarpSyncRequest.GetCommitmentId(), nbr)
	case *nwmodels.Packet_WarpSyncResponse:
		p.handleWarpSyncResponse(packetBody.WarpSyncResponse.GetCommitmentId(), packetBody.WarpSyncResponse.GetBlockIds(), packetBody.WarpSyncResponse.GetMerkleProof(), nbr)
	default:
		return ierrors.Errorf("unsupported packet; packet=%+v, packetBody=%T-%+v", packet, packetBody, packetBody)
	}

	return
}

func (p *Protocol) onBlock(blockData []byte, id network.PeerID) {
	blockIdentifier, err := iotago.BlockIdentifierFromBlockBytes(blockData)
	if err != nil {
		p.Events.Error.Trigger(ierrors.Wrap(err, "failed to deserialize block"), id)
		return
	}

	isNew := p.duplicateBlockBytesFilter.AddIdentifier(types.Identifier(blockIdentifier))

	p.requestedBlockHashesMutex.Lock()
	requested := p.requestedBlockHashes.Delete(types.Identifier(blockIdentifier))
	p.requestedBlockHashesMutex.Unlock()

	if !isNew && !requested {
		return
	}

	block, err := model.BlockFromBytes(blockData, p.apiProvider, serix.WithValidation())
	if err != nil {
		p.Events.Error.Trigger(ierrors.Wrap(err, "failed to deserialize block"), id)
		return
	}

	p.Events.BlockReceived.Trigger(block, id)
}

func (p *Protocol) onBlockRequest(idBytes []byte, id network.PeerID) {
	if len(idBytes) != iotago.BlockIDLength {
		p.Events.Error.Trigger(ierrors.Wrap(iotago.ErrInvalidIdentifierLength, "failed to deserialize block request"), id)

		return
	}

	p.Events.BlockRequestReceived.Trigger(iotago.BlockID(idBytes), id)
}

func (p *Protocol) onSlotCommitment(commitmentBytes []byte, id network.PeerID) {
	receivedCommitment, err := model.CommitmentFromBytes(commitmentBytes, p.apiProvider, serix.WithValidation())
	if err != nil {
		p.Events.Error.Trigger(ierrors.Wrap(err, "failed to deserialize slot commitment"), id)

		return
	}

	p.Events.SlotCommitmentReceived.Trigger(receivedCommitment, id)
}

func (p *Protocol) onSlotCommitmentRequest(idBytes []byte, id network.PeerID) {
	if len(idBytes) != iotago.CommitmentIDLength {
		p.Events.Error.Trigger(ierrors.Wrap(iotago.ErrInvalidIdentifierLength, "failed to deserialize slot commitment request"), id)

		return
	}

	p.Events.SlotCommitmentRequestReceived.Trigger(iotago.CommitmentID(idBytes), id)
}

func (p *Protocol) onAttestations(commitmentBytes []byte, attestationsBytes []byte, merkleProof []byte, id network.PeerID) {
	cm, err := model.CommitmentFromBytes(commitmentBytes, p.apiProvider, serix.WithValidation())
	if err != nil {
		p.Events.Error.Trigger(ierrors.Wrap(err, "failed to deserialize commitment"), id)

		return
	}

	var attestations []*iotago.Attestation
	// TODO: there could be multiple versions of attestations in the same packet
	if _, err := lo.PanicOnErr(p.apiProvider.APIForVersion(iotago.Version(commitmentBytes[0]))).Decode(attestationsBytes, &attestations, serix.WithValidation()); err != nil {
		p.Events.Error.Trigger(ierrors.Wrap(err, "failed to deserialize attestations"), id)

		return
	}

	proof := new(merklehasher.Proof[iotago.Identifier])
	if err := json.Unmarshal(merkleProof, proof); err != nil {
		p.Events.Error.Trigger(ierrors.Wrapf(err, "failed to deserialize merkle proof when receiving attestations for commitment %s", cm.ID()), id)

		return
	}

	p.Events.AttestationsReceived.Trigger(cm, attestations, proof, id)
}

func (p *Protocol) onAttestationsRequest(commitmentIDBytes []byte, id network.PeerID) {
	if len(commitmentIDBytes) != iotago.CommitmentIDLength {
		p.Events.Error.Trigger(ierrors.Wrap(iotago.ErrInvalidIdentifierLength, "failed to deserialize commitmentID in attestations request"), id)

		return
	}

	p.Events.AttestationsRequestReceived.Trigger(iotago.CommitmentID(commitmentIDBytes), id)
}

func newPacket() proto.Message {
	return &nwmodels.Packet{}
}
