package core

import (
	"sync"

	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"

	"github.com/iotaledger/hive.go/ds/bytesfilter"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/ds/types"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/hive.go/serializer/v2/serix"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/network"
	nwmodels "github.com/iotaledger/iota-core/pkg/network/protocols/core/models"
	iotago "github.com/iotaledger/iota.go/v4"
)

const (
	protocolID = "iota-core/0.0.1"
)

type Protocol struct {
	Events *Events

	api iotago.API

	network                   network.Endpoint
	workerPool                *workerpool.WorkerPool
	duplicateBlockBytesFilter *bytesfilter.BytesFilter

	requestedBlockHashes      *shrinkingmap.ShrinkingMap[types.Identifier, types.Empty]
	requestedBlockHashesMutex sync.Mutex
}

func NewProtocol(network network.Endpoint, workerPool *workerpool.WorkerPool, api iotago.API, opts ...options.Option[Protocol]) (protocol *Protocol) {
	return options.Apply(&Protocol{
		Events: NewEvents(),

		network:                   network,
		workerPool:                workerPool,
		api:                       api,
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

func (p *Protocol) SendAttestations(cm *model.Commitment, attestations []*iotago.Attestation, to ...network.PeerID) {
	p.network.Send(&nwmodels.Packet{Body: &nwmodels.Packet_Attestations{Attestations: &nwmodels.Attestations{
		Commitment:   cm.Data(),
		Attestations: lo.PanicOnErr(p.api.Encode(attestations)),
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
			p.onAttestations(packetBody.Attestations.GetCommitment(), packetBody.Attestations.GetAttestations(), nbr)
		})
	case *nwmodels.Packet_AttestationsRequest:
		p.workerPool.Submit(func() {
			p.onAttestationsRequest(packetBody.AttestationsRequest.GetCommitmentId(), nbr)
		})
	default:
		return errors.Errorf("unsupported packet; packet=%+v, packetBody=%T-%+v", packet, packetBody, packetBody)
	}

	return
}

func (p *Protocol) onBlock(blockData []byte, id network.PeerID) {
	blockIdentifier, err := iotago.BlockIdentifierFromBlockBytes(blockData)
	if err != nil {
		p.Events.Error.Trigger(errors.Wrap(err, "failed to deserialize block"), id)
	}

	isNew := p.duplicateBlockBytesFilter.AddIdentifier(types.Identifier(blockIdentifier))

	p.requestedBlockHashesMutex.Lock()
	requested := p.requestedBlockHashes.Delete(types.Identifier(blockIdentifier))
	p.requestedBlockHashesMutex.Unlock()

	if !isNew && !requested {
		return
	}

	block, err := model.BlockFromBytes(blockData, p.api, serix.WithValidation())
	if err != nil {
		p.Events.Error.Trigger(errors.Wrap(err, "failed to deserialize block"), id)
	}

	p.Events.BlockReceived.Trigger(block, id)
}

func (p *Protocol) onBlockRequest(idBytes []byte, id network.PeerID) {
	if len(idBytes) != iotago.BlockIDLength {
		p.Events.Error.Trigger(errors.Wrap(iotago.ErrInvalidIdentifierLength, "failed to deserialize block request"), id)

		return
	}

	p.Events.BlockRequestReceived.Trigger(iotago.BlockID(idBytes), id)
}

func (p *Protocol) onSlotCommitment(commitmentBytes []byte, id network.PeerID) {
	receivedCommitment, err := model.CommitmentFromBytes(commitmentBytes, p.api, serix.WithValidation())
	if err != nil {
		p.Events.Error.Trigger(errors.Wrap(err, "failed to deserialize slot commitment"), id)

		return
	}

	p.Events.SlotCommitmentReceived.Trigger(receivedCommitment, id)
}

func (p *Protocol) onSlotCommitmentRequest(idBytes []byte, id network.PeerID) {
	if len(idBytes) != iotago.CommitmentIDLength {
		p.Events.Error.Trigger(errors.Wrap(iotago.ErrInvalidIdentifierLength, "failed to deserialize slot commitment request"), id)

		return
	}

	p.Events.SlotCommitmentRequestReceived.Trigger(iotago.CommitmentID(idBytes), id)
}

func (p *Protocol) onAttestations(commitmentBytes []byte, attestationsBytes []byte, id network.PeerID) {
	cm, err := model.CommitmentFromBytes(commitmentBytes, p.api, serix.WithValidation())
	if err != nil {
		p.Events.Error.Trigger(errors.Wrap(err, "failed to deserialize commitment"), id)

		return
	}

	var attestations []*iotago.Attestation
	if _, err := p.api.Decode(attestationsBytes, &attestations, serix.WithValidation()); err != nil {
		p.Events.Error.Trigger(errors.Wrap(err, "failed to deserialize attestations"), id)

		return
	}

	p.Events.AttestationsReceived.Trigger(cm, attestations, id)
}

func (p *Protocol) onAttestationsRequest(commitmentIDBytes []byte, id network.PeerID) {
	if len(commitmentIDBytes) != iotago.CommitmentIDLength {
		p.Events.Error.Trigger(errors.Wrap(iotago.ErrInvalidIdentifierLength, "failed to deserialize commitmentID in attestations request"), id)

		return
	}

	p.Events.AttestationsRequestReceived.Trigger(iotago.CommitmentID(commitmentIDBytes), id)
}

func newPacket() proto.Message {
	return &nwmodels.Packet{}
}
