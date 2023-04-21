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

func (p *Protocol) SendBlock(block *iotago.Block, to ...network.PeerID) {
	p.network.Send(&nwmodels.Packet{Body: &nwmodels.Packet_Block{Block: &nwmodels.Block{
		Bytes: lo.PanicOnErr(p.api.Encode(block)),
	}}}, protocolID, to...)
}

func (p *Protocol) RequestBlock(id iotago.BlockID, to ...network.PeerID) {
	p.requestedBlockHashesMutex.Lock()
	p.requestedBlockHashes.Set(types.Identifier(id.Identifier()), types.Void)
	p.requestedBlockHashesMutex.Unlock()

	p.network.Send(&nwmodels.Packet{Body: &nwmodels.Packet_BlockRequest{BlockRequest: &nwmodels.BlockRequest{
		Id: id[:],
	}}}, protocolID, to...)
}

func (p *Protocol) SendSlotCommitment(cm *iotago.Commitment, to ...network.PeerID) {
	p.network.Send(&nwmodels.Packet{Body: &nwmodels.Packet_SlotCommitment{SlotCommitment: &nwmodels.SlotCommitment{
		Bytes: lo.PanicOnErr(p.api.Encode(cm)),
	}}}, protocolID, to...)
}

// func (p *Protocol) SendAttestations(cm *commitment.Commitment, blockIDs models.BlockIDs, attestations *orderedmap.OrderedMap[iotago.SlotIndex, *advancedset.AdvancedSet[*notarization.Attestation]], to ...network.PeerID) {
//	p.network.Send(&nwmodels.Packet{Body: &nwmodels.Packet_Attestations{Attestations: &nwmodels.Attestations{
//		Commitment:   lo.PanicOnErr(cm.Bytes()),
//		BlocksIds:    lo.PanicOnErr(blockIDs.Bytes()),
//		Attestations: lo.PanicOnErr(attestations.Encode()),
//	}}}, protocolID, to...)
// }

func (p *Protocol) RequestCommitment(id iotago.CommitmentID, to ...network.PeerID) {
	p.network.Send(&nwmodels.Packet{Body: &nwmodels.Packet_SlotCommitmentRequest{SlotCommitmentRequest: &nwmodels.SlotCommitmentRequest{
		Id: id[:],
	}}}, protocolID, to...)
}

func (p *Protocol) RequestAttestations(cm *iotago.Commitment, endIndex iotago.SlotIndex, to ...network.PeerID) {
	p.network.Send(&nwmodels.Packet{Body: &nwmodels.Packet_AttestationsRequest{AttestationsRequest: &nwmodels.AttestationsRequest{
		Commitment: lo.PanicOnErr(p.api.Encode(cm)),
		EndIndex:   endIndex.Bytes(),
	}}}, protocolID, to...)
}

func (p *Protocol) Unregister() {
	p.network.UnregisterProtocol(protocolID)
}

func (p *Protocol) handlePacket(nbr network.PeerID, packet proto.Message) (err error) {
	switch packetBody := packet.(*nwmodels.Packet).GetBody().(type) {
	case *nwmodels.Packet_Block:
		p.workerPool.Submit(func() { p.onBlock(packetBody.Block.GetBytes(), nbr) })
	case *nwmodels.Packet_BlockRequest:
		p.workerPool.Submit(func() { p.onBlockRequest(packetBody.BlockRequest.GetId(), nbr) })
	case *nwmodels.Packet_SlotCommitment:
		p.workerPool.Submit(func() { p.onSlotCommitment(packetBody.SlotCommitment.GetBytes(), nbr) })
	case *nwmodels.Packet_SlotCommitmentRequest:
		p.workerPool.Submit(func() { p.onSlotCommitmentRequest(packetBody.SlotCommitmentRequest.GetId(), nbr) })
	case *nwmodels.Packet_Attestations:
		// p.workerPool.Submit(func() {
		//	p.onAttestations(packetBody.Attestations.GetCommitment(), packetBody.Attestations.GetBlocksIds(), packetBody.Attestations.GetAttestations(), nbr)
		// })
	case *nwmodels.Packet_AttestationsRequest:
		p.workerPool.Submit(func() {
			p.onAttestationsRequest(packetBody.AttestationsRequest.GetCommitment(), packetBody.AttestationsRequest.GetEndIndex(), nbr)
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
	receivedCommitment := new(iotago.Commitment)
	if _, err := p.api.Decode(commitmentBytes, receivedCommitment, serix.WithValidation()); err != nil {
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

// func (p *Protocol) onAttestations(commitmentBytes []byte, blockIDBytes []byte, attestationsBytes []byte, id network.PeerID) {
//	cm := &commitment.Commitment{}
//	if _, err := cm.FromBytes(commitmentBytes); err != nil {
//		p.Events.Error.Trigger(&ErrorEvent{
//			Error:  errors.Wrap(err, "failed to deserialize commitment"),
//			Source: id,
//		})
//
//		return
//	}
//
//	blockIDs := models.NewBlockIDs()
//	if _, err := blockIDs.FromBytes(blockIDBytes); err != nil {
//		p.Events.Error.Trigger(&ErrorEvent{
//			Error:  errors.Wrap(err, "failed to deserialize blockIDs"),
//			Source: id,
//		})
//
//		return
//	}
//
//	attestations := orderedmap.New[iotago.SlotIndex, *advancedset.AdvancedSet[*notarization.Attestation]]()
//	if _, err := attestations.Decode(attestationsBytes); err != nil {
//		p.Events.Error.Trigger(&ErrorEvent{
//			Error:  errors.Wrap(err, "failed to deserialize attestations"),
//			Source: id,
//		})
//
//		return
//	}
//
//	p.Events.AttestationsReceived.Trigger(&AttestationsReceivedEvent{
//		Commitment:   cm,
//		BlockIDs:     blockIDs,
//		Attestations: attestations,
//		Source:       id,
//	})
// }

func (p *Protocol) onAttestationsRequest(commitmentBytes []byte, slotIndexBytes []byte, id network.PeerID) {
	cm := new(iotago.Commitment)
	if _, err := p.api.Decode(commitmentBytes, cm, serix.WithValidation()); err != nil {
		p.Events.Error.Trigger(errors.Wrap(err, "failed to deserialize commitment"), id)

		return
	}

	endSlotIndex, err := iotago.SlotIndexFromBytes(slotIndexBytes)
	if err != nil {
		p.Events.Error.Trigger(errors.Wrap(err, "failed to deserialize end iotago.SlotIndex"), id)

		return
	}

	p.Events.AttestationsRequestReceived.Trigger(&AttestationsRequestReceivedEvent{
		Commitment: cm,
		EndIndex:   endSlotIndex,
		Source:     id,
	})
}

func newPacket() proto.Message {
	return &nwmodels.Packet{}
}
