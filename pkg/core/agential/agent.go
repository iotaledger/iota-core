package agential

type Agent interface {
	OnConstructed(handler func()) (unsubscribe func())
}
