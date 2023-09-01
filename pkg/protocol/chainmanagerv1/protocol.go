package chainmanagerv1

type Protocol struct {
	*EngineManager
	*ChainManager
}

func NewProtocol() *Protocol {
	p := &Protocol{
		EngineManager: NewEngineManager(p),
		ChainManager:  NewChainManager(),
	}
	return p
}
