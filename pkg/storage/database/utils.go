package database

func FlushAndClose(store *syncedKVStore) error {
	if err := store.FlushWithoutLocking(); err != nil {
		return err
	}

	return store.CloseWithoutLocking()
}
