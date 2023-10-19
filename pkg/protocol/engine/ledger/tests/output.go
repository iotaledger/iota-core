package ledgertests

import iotago "github.com/iotaledger/iota.go/v4"

type MockedOutput struct{}

func (m *MockedOutput) Equal(_ iotago.Output) bool {
	panic("implement me")
}

func (m *MockedOutput) Size() int {
	panic("implement me")
}

func (m *MockedOutput) WorkScore(_ *iotago.WorkScoreParameters) (iotago.WorkScore, error) {
	panic("implement me")
}

func (m *MockedOutput) StorageScore(_ *iotago.StorageScoreStructure, _ iotago.StorageScoreFunc) iotago.StorageScore {
	panic("implement me")
}

func (m *MockedOutput) BaseTokenAmount() iotago.BaseToken {
	panic("implement me")
}

func (m *MockedOutput) StoredMana() iotago.Mana {
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
