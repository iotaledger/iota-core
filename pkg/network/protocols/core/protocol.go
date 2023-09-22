package core

import (
	"encoding/binary"
	"encoding/json"

	"github.com/libp2p/go-libp2p/core/peer"
	"google.golang.org/protobuf/proto"

	"github.com/iotaledger/hive.go/ds/bytesfilter"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/ds/types"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/hive.go/serializer/v2/marshalutil"
	"github.com/iotaledger/hive.go/serializer/v2/serix"
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
	duplicateBlockBytesFilter *bytesfilter.BytesFilter

	requestedBlockHashes      *shrinkingmap.ShrinkingMap[types.Identifier, types.Empty]
	requestedBlockHashesMutex syncutils.Mutex
}

func NewProtocol(network network.Endpoint, workerPool *workerpool.WorkerPool, apiProvider iotago.APIProvider, opts ...options.Option[Protocol]) (protocol *Protocol) {
	return options.Apply(&Protocol{
		Events: NewEvents(),

		network:                   network,
		workerPool:                workerPool,
		apiProvider:               apiProvider,
		duplicateBlockBytesFilter: bytesfilter.New(10000),
		requestedBlockHashes:      shrinkingmap.New[types.Identifier, types.Empty](shrinkingmap.WithShrinkingThresholdCount(1000)),
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
	p.requestedBlockHashes.Set(types.Identifier(id.Identifier()), types.Void)
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

func (p *Protocol) SendAttestations(cm *model.Commitment, attestations []*iotago.Attestation, merkleProof *merklehasher.Proof[iotago.Identifier], to ...peer.ID) {
	encodedAttestations := marshalutil.New()
	encodedAttestations.WriteUint32(uint32(len(attestations)))
	for _, att := range attestations {
		iotagoAPI := lo.PanicOnErr(p.apiProvider.APIForVersion(att.ProtocolVersion))
		encodedAttestations.WriteBytes(lo.PanicOnErr(iotagoAPI.Encode(att)))
	}

	p.network.Send(&nwmodels.Packet{Body: &nwmodels.Packet_Attestations{Attestations: &nwmodels.Attestations{
		Commitment:   cm.Data(),
		Attestations: encodedAttestations.Bytes(),
		MerkleProof:  lo.PanicOnErr(json.Marshal(merkleProof)),
	}}}, to...)
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

func (p *Protocol) Shutdown() {
	p.network.Shutdown()

	p.workerPool.Shutdown()
	p.workerPool.ShutdownComplete.Wait()
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
		p.handleWarpSyncResponse(packetBody.WarpSyncResponse.GetCommitmentId(), packetBody.WarpSyncResponse.GetBlockIds(), packetBody.WarpSyncResponse.GetMerkleProof(), nbr)
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

func (p *Protocol) onBlockRequest(idBytes []byte, id peer.ID) {
	if len(idBytes) != iotago.BlockIDLength {
		p.Events.Error.Trigger(ierrors.Wrap(iotago.ErrInvalidIdentifierLength, "failed to deserialize block request"), id)

		return
	}

	p.Events.BlockRequestReceived.Trigger(iotago.BlockID(idBytes), id)
}

func (p *Protocol) onSlotCommitment(commitmentBytes []byte, id peer.ID) {
	receivedCommitment, err := model.CommitmentFromBytes(commitmentBytes, p.apiProvider, serix.WithValidation())
	if err != nil {
		p.Events.Error.Trigger(ierrors.Wrap(err, "failed to deserialize slot commitment"), id)

		return
	}

	p.Events.SlotCommitmentReceived.Trigger(receivedCommitment, id)
}

func (p *Protocol) onSlotCommitmentRequest(idBytes []byte, id peer.ID) {
	if len(idBytes) != iotago.CommitmentIDLength {
		p.Events.Error.Trigger(ierrors.Wrap(iotago.ErrInvalidIdentifierLength, "failed to deserialize slot commitment request"), id)

		return
	}

	p.Events.SlotCommitmentRequestReceived.Trigger(iotago.CommitmentID(idBytes), id)
}

func (p *Protocol) onAttestations(commitmentBytes []byte, attestationsBytes []byte, merkleProof []byte, id peer.ID) {
	cm, err := model.CommitmentFromBytes(commitmentBytes, p.apiProvider, serix.WithValidation())
	if err != nil {
		p.Events.Error.Trigger(ierrors.Wrap(err, "failed to deserialize commitment"), id)

		return
	}

	if len(attestationsBytes) < 4 {
		p.Events.Error.Trigger(ierrors.Errorf("failed to deserialize attestations, invalid attestation count"), id)

		return
	}

	attestationCount := binary.LittleEndian.Uint32(attestationsBytes[0:4])
	readOffset := 4
	attestations := make([]*iotago.Attestation, attestationCount)
	for i := uint32(0); i < attestationCount; i++ {
		attestation, consumed, err := iotago.AttestationFromBytes(p.apiProvider)(attestationsBytes[readOffset:])
		if err != nil {
			p.Events.Error.Trigger(ierrors.Wrap(err, "failed to deserialize attestations"), id)

			return
		}

		readOffset += consumed
		attestations[i] = attestation
	}
	if readOffset != len(attestationsBytes) {
		p.Events.Error.Trigger(ierrors.Errorf("failed to deserialize attestations: %d bytes remaining", len(attestationsBytes)-readOffset), id)

		return
	}

	proof := new(merklehasher.Proof[iotago.Identifier])
	if err := json.Unmarshal(merkleProof, proof); err != nil {
		p.Events.Error.Trigger(ierrors.Wrapf(err, "failed to deserialize merkle proof when receiving attestations for commitment %s", cm.ID()), id)

		return
	}

	p.Events.AttestationsReceived.Trigger(cm, attestations, proof, id)
}

func (p *Protocol) onAttestationsRequest(commitmentIDBytes []byte, id peer.ID) {
	if len(commitmentIDBytes) != iotago.CommitmentIDLength {
		p.Events.Error.Trigger(ierrors.Wrap(iotago.ErrInvalidIdentifierLength, "failed to deserialize commitmentID in attestations request"), id)

		return
	}

	p.Events.AttestationsRequestReceived.Trigger(iotago.CommitmentID(commitmentIDBytes), id)
}

func newPacket() proto.Message {
	return &nwmodels.Packet{}
}
