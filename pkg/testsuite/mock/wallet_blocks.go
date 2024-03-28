package mock

import (
	"context"

	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
)

func (w *Wallet) CreateBasicBlock(ctx context.Context, blockName string, opts ...options.Option[BasicBlockParams]) (*blocks.Block, error) {
	return w.BlockIssuer.CreateBasicBlock(ctx, blockName, opts...)
}

func (w *Wallet) CreateAndSubmitBasicBlock(ctx context.Context, blockName string, opts ...options.Option[BasicBlockParams]) (*blocks.Block, error) {
	return w.BlockIssuer.CreateAndSubmitBasicBlock(ctx, blockName, opts...)
}
