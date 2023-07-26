//go:build ignore

package inx

import iotago "github.com/iotaledger/iota.go/v4"

type streamRange struct {
	start    iotago.SlotIndex
	end      iotago.SlotIndex
	lastSent iotago.SlotIndex
}

// tells whether the stream range has a range requested.
func (stream *streamRange) rangeRequested() bool {
	return stream.start > 0
}

// tells whether the stream is bounded, aka has an end index.
func (stream *streamRange) isBounded() bool {
	return stream.end > 0
}

// handleRangedSend {{- if hasParams}}{{paramCount}}{{end}} handles the sending of data within a streamRange.
//   - sendFunc gets executed for the given index.
//   - if data wasn't sent between streamRange.lastSent and the given index, then the given catchUpFunc is executed
//     with the range from streamRange.lastSent + 1 up to index - 1.
//   - streamRange.lastSent is auto. updated
//
//go:generate go run github.com/iotaledger/hive.go/codegen/variadic/cmd@latest 1 2 rangesend.go
func handleRangedSend /*{{- if hasParams}}{{paramCount}}{{"["}}{{types}}{{" any]"}}{{end -}}*/ (index iotago.SlotIndex /*{{- ", "}}{{typedParams -}}*/, streamRange *streamRange,
	catchUpFunc func(start iotago.SlotIndex, end iotago.SlotIndex) error,
	sendFunc func(index iotago.SlotIndex /*{{- ", "}}{{typedParams -}}*/) error,
) (bool, error) {

	// below requested range
	if streamRange.rangeRequested() && index < streamRange.start {
		return false, nil
	}

	// execute catch up function with missing indices
	if streamRange.rangeRequested() && index-1 > streamRange.lastSent {
		startIndex := streamRange.start
		if startIndex < streamRange.lastSent+1 {
			startIndex = streamRange.lastSent + 1
		}

		endIndex := index - 1
		if streamRange.isBounded() && endIndex > streamRange.end {
			endIndex = streamRange.end
		}

		if err := catchUpFunc(startIndex, endIndex); err != nil {
			return false, err
		}

		streamRange.lastSent = endIndex
	}

	// stream finished
	if streamRange.isBounded() && index > streamRange.end {
		return true, nil
	}

	if err := sendFunc(index /*{{- ", "}}{{params -}}*/); err != nil {
		return false, err
	}

	streamRange.lastSent = index

	// stream finished
	if streamRange.isBounded() && index >= streamRange.end {
		return true, nil
	}

	return false, nil
}
