package core

import (
	"github.com/libp2p/go-libp2p/core/peer"
	"google.golang.org/protobuf/proto"

	"github.com/iotaledger/hive.go/ds/bytesfilter"
	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/ds/types"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/hive.go/serializer/v2"
	"github.com/iotaledger/hive.go/serializer/v2/stream"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/network"
	nwmodels "github.com/iotaledger/iota-core/pkg/network/protocols/core/models"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/merklehasher"
)

type Protocol struct {
	Events *Events

	apiProvider iotago.APIProvider

	network                   network.Endpoint
	workerPool                *workerpool.WorkerPool
	duplicateBlockBytesFilter *bytesfilter.BytesFilter[iotago.Identifier]

	requestedBlockHashes      *shrinkingmap.ShrinkingMap[iotago.Identifier, types.Empty]
	requestedBlockHashesMutex syncutils.Mutex

	shutdown reactive.Event
}

func NewProtocol(network network.Endpoint, workerPool *workerpool.WorkerPool, apiProvider iotago.APIProvider, opts ...options.Option[Protocol]) (protocol *Protocol) {
	return options.Apply(&Protocol{
		Events: NewEvents(),

		network:                   network,
		workerPool:                workerPool,
		apiProvider:               apiProvider,
		duplicateBlockBytesFilter: bytesfilter.New(iotago.IdentifierFromData, 10000),
		requestedBlockHashes:      shrinkingmap.New[iotago.Identifier, types.Empty](shrinkingmap.WithShrinkingThresholdCount(1000)),
		shutdown:                  reactive.NewEvent(),
	}, opts, func(p *Protocol) {
		network.RegisterProtocol(newPacket, p.handlePacket)
	})
}

func (p *Protocol) SendBlock(block *model.Block, to ...peer.ID) {
	p.network.Send(&nwmodels.Packet{Body: &nwmodels.Packet_Block{Block: &nwmodels.Block{
		Bytes: block.Data(),
	}}}, to...)
}

func (p *Protocol) RequestBlock(id iotago.BlockID, to ...peer.ID) {
	p.requestedBlockHashesMutex.Lock()
	p.requestedBlockHashes.Set(id.Identifier(), types.Void)
	p.requestedBlockHashesMutex.Unlock()

	p.network.Send(&nwmodels.Packet{Body: &nwmodels.Packet_BlockRequest{BlockRequest: &nwmodels.BlockRequest{
		BlockId: id[:],
	}}}, to...)
}

func (p *Protocol) SendSlotCommitment(cm *model.Commitment, to ...peer.ID) {
	p.network.Send(&nwmodels.Packet{Body: &nwmodels.Packet_SlotCommitment{SlotCommitment: &nwmodels.SlotCommitment{
		Bytes: cm.Data(),
	}}}, to...)
}

func (p *Protocol) SendAttestations(cm *model.Commitment, attestations []*iotago.Attestation, merkleProof *merklehasher.Proof[iotago.Identifier], to ...peer.ID) error {
	byteBuffer := stream.NewByteBuffer()

	if err := stream.WriteCollection(byteBuffer, serializer.SeriLengthPrefixTypeAsUint32, func() (elementsCount int, err error) {
		for _, att := range attestations {
			if err = stream.WriteObjectWithSize(byteBuffer, att, serializer.SeriLengthPrefixTypeAsUint16, (*iotago.Attestation).Bytes); err != nil {
				return 0, ierrors.Wrapf(err, "failed to write attestation %v", att)
			}
		}

		return len(attestations), nil
	}); err != nil {
		return err
	}

	p.network.Send(&nwmodels.Packet{Body: &nwmodels.Packet_Attestations{Attestations: &nwmodels.Attestations{
		Commitment:   cm.Data(),
		Attestations: lo.PanicOnErr(byteBuffer.Bytes()),
		MerkleProof:  lo.PanicOnErr(merkleProof.Bytes()),
	}}}, to...)

	return nil
}

func (p *Protocol) RequestSlotCommitment(id iotago.CommitmentID, to ...peer.ID) {
	p.network.Send(&nwmodels.Packet{Body: &nwmodels.Packet_SlotCommitmentRequest{SlotCommitmentRequest: &nwmodels.SlotCommitmentRequest{
		CommitmentId: id[:],
	}}}, to...)
}

func (p *Protocol) RequestAttestations(id iotago.CommitmentID, to ...peer.ID) {
	p.network.Send(&nwmodels.Packet{Body: &nwmodels.Packet_AttestationsRequest{AttestationsRequest: &nwmodels.AttestationsRequest{
		CommitmentId: lo.PanicOnErr(id.Bytes()),
	}}}, to...)
}

func (p *Protocol) OnBlockReceived(callback func(block *model.Block, src peer.ID)) (unsubscribe func()) {
	return p.Events.BlockReceived.Hook(callback).Unhook
}

func (p *Protocol) OnBlockRequestReceived(callback func(blockID iotago.BlockID, src peer.ID)) (unsubscribe func()) {
	return p.Events.BlockRequestReceived.Hook(callback).Unhook
}

func (p *Protocol) OnCommitmentReceived(callback func(commitment *model.Commitment, src peer.ID)) (unsubscribe func()) {
	return p.Events.SlotCommitmentReceived.Hook(callback).Unhook
}

func (p *Protocol) OnCommitmentRequestReceived(callback func(commitmentID iotago.CommitmentID, src peer.ID)) (unsubscribe func()) {
	return p.Events.SlotCommitmentRequestReceived.Hook(callback).Unhook
}

func (p *Protocol) OnAttestationsReceived(callback func(*model.Commitment, []*iotago.Attestation, *merklehasher.Proof[iotago.Identifier], peer.ID)) (unsubscribe func()) {
	return p.Events.AttestationsReceived.Hook(callback).Unhook
}

func (p *Protocol) OnAttestationsRequestReceived(callback func(commitmentID iotago.CommitmentID, src peer.ID)) (unsubscribe func()) {
	return p.Events.AttestationsRequestReceived.Hook(callback).Unhook
}

func (p *Protocol) OnWarpSyncResponseReceived(callback func(commitmentID iotago.CommitmentID, blockIDs map[iotago.CommitmentID]iotago.BlockIDs, proof *merklehasher.Proof[iotago.Identifier], transactionIDs iotago.TransactionIDs, mutationProof *merklehasher.Proof[iotago.Identifier], src peer.ID)) (unsubscribe func()) {
	return p.Events.WarpSyncResponseReceived.Hook(callback).Unhook
}

func (p *Protocol) OnWarpSyncRequestReceived(callback func(commitmentID iotago.CommitmentID, src peer.ID)) (unsubscribe func()) {
	return p.Events.WarpSyncRequestReceived.Hook(callback).Unhook
}

func (p *Protocol) OnError(callback func(err error, src peer.ID)) (unsubscribe func()) {
	return p.Events.Error.Hook(callback).Unhook
}

func (p *Protocol) Shutdown() {
	p.network.Shutdown()

	p.workerPool.Shutdown()
	p.workerPool.ShutdownComplete.Wait()

	p.shutdown.Trigger()
}

func (p *Protocol) OnShutdown(callback func()) (unsubscribe func()) {
	return p.shutdown.OnTrigger(callback)
}

func (p *Protocol) handlePacket(nbr peer.ID, packet proto.Message) (err error) {
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
		p.handleWarpSyncResponse(packetBody.WarpSyncResponse.GetCommitmentId(), packetBody.WarpSyncResponse.GetPayload(), nbr)
	default:
		return ierrors.Errorf("unsupported packet; packet=%+v, packetBody=%T-%+v", packet, packetBody, packetBody)
	}

	return
}

func (p *Protocol) onBlock(blockData []byte, id peer.ID) {
	blockIdentifier, err := iotago.BlockIdentifierFromBlockBytes(blockData)
	if err != nil {
		p.Events.Error.Trigger(ierrors.Wrap(err, "failed to deserialize block"), id)
		return
	}

	isNew := p.duplicateBlockBytesFilter.AddIdentifier(blockIdentifier)

	p.requestedBlockHashesMutex.Lock()
	requested := p.requestedBlockHashes.Delete(blockIdentifier)
	p.requestedBlockHashesMutex.Unlock()

	if !isNew && !requested {
		return
	}

	block, err := model.BlockFromBlockIdentifierAndBytes(blockIdentifier, blockData, p.apiProvider)
	if err != nil {
		p.Events.Error.Trigger(ierrors.Wrap(err, "failed to deserialize block"), id)
		return
	}

	p.Events.BlockReceived.Trigger(block, id)
}

func (p *Protocol) onBlockRequest(idBytes []byte, id peer.ID) {
	if len(idBytes) != iotago.BlockIDLength {
		p.Events.Error.Trigger(ierrors.New("failed to deserialize block request: invalid block id length"), id)

		return
	}

	p.Events.BlockRequestReceived.Trigger(iotago.BlockID(idBytes), id)
}

func (p *Protocol) onSlotCommitment(commitmentBytes []byte, id peer.ID) {
	receivedCommitment, err := lo.DropCount(model.CommitmentFromBytes(p.apiProvider)(commitmentBytes))
	if err != nil {
		p.Events.Error.Trigger(ierrors.Wrap(err, "failed to deserialize slot commitment"), id)

		return
	}

	p.Events.SlotCommitmentReceived.Trigger(receivedCommitment, id)
}

func (p *Protocol) onSlotCommitmentRequest(idBytes []byte, id peer.ID) {
	if len(idBytes) != iotago.CommitmentIDLength {
		p.Events.Error.Trigger(ierrors.New("failed to deserialize slot commitment request: invalid commitment id length"), id)

		return
	}

	p.Events.SlotCommitmentRequestReceived.Trigger(iotago.CommitmentID(idBytes), id)
}

func (p *Protocol) onAttestations(commitmentBytes []byte, attestationsBytes []byte, merkleProof []byte, id peer.ID) {
	cm, err := lo.DropCount(model.CommitmentFromBytes(p.apiProvider)(commitmentBytes))
	if err != nil {
		p.Events.Error.Trigger(ierrors.Wrap(err, "failed to deserialize commitment"), id)

		return
	}

	reader := stream.NewByteReader(attestationsBytes)

	attestationsCount, err := stream.PeekSize(reader, serializer.SeriLengthPrefixTypeAsUint32)
	if err != nil {
		p.Events.Error.Trigger(ierrors.New("failed peek attestations count"), id)

		return
	}

	attestations := make([]*iotago.Attestation, attestationsCount)
	if err := stream.ReadCollection(reader, serializer.SeriLengthPrefixTypeAsUint32, func(i int) error {
		attestations[i], err = stream.ReadObjectWithSize(reader, serializer.SeriLengthPrefixTypeAsUint16, iotago.AttestationFromBytes(p.apiProvider))
		if err != nil {
			return ierrors.Wrapf(err, "failed to deserialize attestation %d", i)
		}

		return nil
	}); err != nil {
		p.Events.Error.Trigger(ierrors.Wrap(err, "failed to deserialize attestations"), id)

		return
	}

	if reader.BytesRead() != len(attestationsBytes) {
		p.Events.Error.Trigger(ierrors.Errorf("failed to deserialize attestations: %d bytes remaining", len(attestationsBytes)-reader.BytesRead()), id)

		return
	}

	proof, _, err := merklehasher.ProofFromBytes[iotago.Identifier](merkleProof)
	if err != nil {
		p.Events.Error.Trigger(ierrors.Wrapf(err, "failed to deserialize merkle proof when receiving attestations for commitment %s", cm.ID()), id)

		return
	}

	p.Events.AttestationsReceived.Trigger(cm, attestations, proof, id)
}

func (p *Protocol) onAttestationsRequest(commitmentIDBytes []byte, id peer.ID) {
	if len(commitmentIDBytes) != iotago.CommitmentIDLength {
		p.Events.Error.Trigger(ierrors.New("failed to deserialize commitmentID in attestations request: invalid commitment id length"), id)

		return
	}

	p.Events.AttestationsRequestReceived.Trigger(iotago.CommitmentID(commitmentIDBytes), id)
}

func newPacket() proto.Message {
	return &nwmodels.Packet{}
}
