package database

func FlushAndClose(store *lockedKVStore) error {
	if err := store.FlushWithoutLocking(); err != nil {
		return err
	}

	return store.CloseWithoutLocking()
}
