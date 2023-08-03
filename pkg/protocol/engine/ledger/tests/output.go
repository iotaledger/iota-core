package ledgertests

import iotago "github.com/iotaledger/iota.go/v4"

type MockedOutput struct{}

func (m *MockedOutput) Size() int {
	panic("implement me")
}

func (m *MockedOutput) WorkScore(_ *iotago.WorkScoreStructure) (iotago.WorkScore, error) {
	panic("implement me")
}

func (m *MockedOutput) VBytes(_ *iotago.RentStructure, _ iotago.VBytesFunc) iotago.VBytes {
	panic("implement me")
}

func (m *MockedOutput) Deposit() iotago.BaseToken {
	panic("implement me")
}

func (m *MockedOutput) StoredMana() iotago.Mana {
	panic("implement me")
}

func (m *MockedOutput) NativeTokenList() iotago.NativeTokens {
	panic("implement me")
}

func (m *MockedOutput) UnlockConditionSet() iotago.UnlockConditionSet {
	panic("implement me")
}

func (m *MockedOutput) FeatureSet() iotago.FeatureSet {
	panic("implement me")
}

func (m *MockedOutput) Type() iotago.OutputType {
	panic("implement me")
}

func (m *MockedOutput) Clone() iotago.Output {
	panic("implement me")
}
