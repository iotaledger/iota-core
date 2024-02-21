package network

import "github.com/iotaledger/hive.go/ierrors"

var (
	// ErrNotRunning is returned when a peer is added to a stopped or not yet started network manager.
	ErrNotRunning = ierrors.New("manager not running")
	// ErrUnknownPeer is returned when the specified peer is not known to the network manager.
	ErrUnknownPeer = ierrors.New("unknown neighbor")
	// ErrLoopbackPeer is returned when the own peer is added.
	ErrLoopbackPeer = ierrors.New("loopback connection not allowed")
	// ErrDuplicatePeer is returned when the same peer is added more than once.
	ErrDuplicatePeer = ierrors.New("already connected")
)
