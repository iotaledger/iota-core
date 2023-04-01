package devnetvm

import (
	"context"

	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/serializer/v2/serix"
	"github.com/iotaledger/iota-core/pkg/cerrors"
	"github.com/iotaledger/iota-core/pkg/utxo"
)

// OutputFromBytes is the factory function for Outputs.
func OutputFromBytes(data []byte) (output utxo.Output, err error) {
	_, err = serix.DefaultAPI.Decode(context.Background(), data, &output, serix.WithValidation())
	if err != nil {
		return nil, errors.WithMessagef(cerrors.ErrParseBytesFailed, "failed to parse Output: %s", err.Error())
	}

	return output, nil
}
