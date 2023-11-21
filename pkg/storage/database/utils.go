package database

func FlushAndClose(store *lockedKVStore) error {
	if err := store.instance().Flush(); err != nil {
		return err
	}

	return store.instance().Close()
}
