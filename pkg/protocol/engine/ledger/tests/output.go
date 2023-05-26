package ledgertests

import iotago "github.com/iotaledger/iota.go/v4"

type MockedOutput struct{}

func (m *MockedOutput) Size() int {
	//TODO implement me
	panic("implement me")
}

func (m *MockedOutput) VBytes(_ *iotago.RentStructure, _ iotago.VBytesFunc) iotago.VBytes {
	//TODO implement me
	panic("implement me")
}

func (m *MockedOutput) Deposit() uint64 {
	//TODO implement me
	panic("implement me")
}

func (m *MockedOutput) NativeTokenList() iotago.NativeTokens {
	//TODO implement me
	panic("implement me")
}

func (m *MockedOutput) UnlockConditionSet() iotago.UnlockConditionSet {
	//TODO implement me
	panic("implement me")
}

func (m *MockedOutput) FeatureSet() iotago.FeatureSet {
	//TODO implement me
	panic("implement me")
}

func (m *MockedOutput) Type() iotago.OutputType {
	//TODO implement me
	panic("implement me")
}

func (m *MockedOutput) Clone() iotago.Output {
	//TODO implement me
	panic("implement me")
}
