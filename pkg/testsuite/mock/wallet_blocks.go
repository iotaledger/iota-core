package mock

import (
	"context"

	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
)

func (w *Wallet) IssueBasicBlock(ctx context.Context, blockName string, opts ...options.Option[BasicBlockParams]) (*blocks.Block, error) {
	block, err := w.BlockIssuer.IssueBasicBlock(ctx, blockName, opts...)

	return block, err
}
