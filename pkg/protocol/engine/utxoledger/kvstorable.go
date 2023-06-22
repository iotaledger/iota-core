package utxoledger

type kvStorable interface {
	KVStorableKey() (key []byte)
	KVStorableValue() (value []byte)
	kvStorableLoad(manager *Manager, key []byte, value []byte) error
}
