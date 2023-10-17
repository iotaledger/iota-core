package models

import (
	"context"
	"time"

	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/iota-core/pkg/model"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/builder"
	"github.com/iotaledger/iota.go/v4/nodeclient"
	"github.com/iotaledger/iota.go/v4/nodeclient/apimodels"
)

type ServerInfo struct {
	Healthy bool
	Version string
}

type ServerInfos []*ServerInfo

type Connector interface {
	// ServersStatuses retrieves the connected server status for each client.
	ServersStatuses() ServerInfos
	// ServerStatus retrieves the connected server status.
	ServerStatus(cltIdx int) (status *ServerInfo, err error)
	// Clients returns list of all clients.
	Clients(...bool) []Client
	// GetClients returns the numOfClt client instances that were used the longest time ago.
	GetClients(numOfClt int) []Client
	// AddClient adds a client to WebClients based on provided GoShimmerAPI url.
	AddClient(url string, setters ...options.Option[WebClient])
	// RemoveClient removes a client with the provided url from the WebClients.
	RemoveClient(url string)
	// GetClient returns the client instance that was used the longest time ago.
	GetClient() Client
}

// WebClients is responsible for handling connections via GoShimmerAPI.
type WebClients struct {
	clients []*WebClient
	urls    []string

	// helper variable indicating which clt was recently used, useful for double, triple,... spends
	lastUsed int

	mu syncutils.Mutex
}

// NewWebClients creates Connector from provided GoShimmerAPI urls.
func NewWebClients(urls []string, setters ...options.Option[WebClient]) *WebClients {
	clients := make([]*WebClient, len(urls))
	for i, url := range urls {
		clients[i] = NewWebClient(url, setters...)
	}

	return &WebClients{
		clients:  clients,
		urls:     urls,
		lastUsed: -1,
	}
}

// ServersStatuses retrieves the connected server status for each client.
func (c *WebClients) ServersStatuses() ServerInfos {
	status := make(ServerInfos, len(c.clients))

	for i := range c.clients {
		status[i], _ = c.ServerStatus(i)
	}

	return status
}

// ServerStatus retrieves the connected server status.
func (c *WebClients) ServerStatus(cltIdx int) (status *ServerInfo, err error) {
	response, err := c.clients[cltIdx].client.Info(context.Background())
	if err != nil {
		return nil, err
	}

	return &ServerInfo{
		Healthy: response.Status.IsHealthy,
		Version: response.Version,
	}, nil
}

// Clients returns list of all clients.
func (c *WebClients) Clients(...bool) []Client {
	clients := make([]Client, len(c.clients))
	for i, c := range c.clients {
		clients[i] = c
	}

	return clients
}

// GetClients returns the numOfClt client instances that were used the longest time ago.
func (c *WebClients) GetClients(numOfClt int) []Client {
	c.mu.Lock()
	defer c.mu.Unlock()

	clts := make([]Client, numOfClt)

	for i := range clts {
		clts[i] = c.getClient()
	}

	return clts
}

// getClient returns the client instance that was used the longest time ago, not protected by mutex.
func (c *WebClients) getClient() Client {
	if c.lastUsed >= len(c.clients)-1 {
		c.lastUsed = 0
	} else {
		c.lastUsed++
	}

	return c.clients[c.lastUsed]
}

// GetClient returns the client instance that was used the longest time ago.
func (c *WebClients) GetClient() Client {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.getClient()
}

// AddClient adds client to WebClients based on provided GoShimmerAPI url.
func (c *WebClients) AddClient(url string, setters ...options.Option[WebClient]) {
	c.mu.Lock()
	defer c.mu.Unlock()

	clt := NewWebClient(url, setters...)
	c.clients = append(c.clients, clt)
	c.urls = append(c.urls, url)
}

// RemoveClient removes client with the provided url from the WebClients.
func (c *WebClients) RemoveClient(url string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	indexToRemove := -1
	for i, u := range c.urls {
		if u == url {
			indexToRemove = i
			break
		}
	}
	if indexToRemove == -1 {
		return
	}
	c.clients = append(c.clients[:indexToRemove], c.clients[indexToRemove+1:]...)
	c.urls = append(c.urls[:indexToRemove], c.urls[indexToRemove+1:]...)
}

type Client interface {
	Client() *nodeclient.Client
	// URL returns a client API url.
	URL() (cltID string)
	// PostBlock sends a block to the Tangle via a given client.
	PostBlock(block *iotago.ProtocolBlock) (iotago.BlockID, error)
	// PostData sends the given data (payload) by creating a block in the backend.
	PostData(data []byte) (blkID string, err error)
	// GetTransactionConfirmationState returns the AcceptanceState of a given transaction ID.
	GetTransactionConfirmationState(txID iotago.TransactionID) string
	// GetOutput gets the output of a given outputID.
	GetOutput(outputID iotago.OutputID) iotago.Output
	// GetOutputConfirmationState gets the first unspent outputs of a given address.
	GetOutputConfirmationState(outputID iotago.OutputID) string
	// GetTransaction gets the transaction.
	GetTransaction(txID iotago.TransactionID) (resp *iotago.SignedTransaction, err error)
	// GetBlockIssuance returns the latest commitment and data needed to create a new block.
	GetBlockIssuance() (resp *apimodels.IssuanceBlockHeaderResponse, err error)
	// GetCongestion returns congestion data such as rmc or issuing readiness.
	GetCongestion(id iotago.AccountID) (resp *apimodels.CongestionResponse, err error)

	iotago.APIProvider
}

// WebClient contains a GoShimmer web API to interact with a node.
type WebClient struct {
	client *nodeclient.Client
	url    string
}

func (c *WebClient) Client() *nodeclient.Client {
	return c.client
}

func (c *WebClient) APIForVersion(version iotago.Version) (iotago.API, error) {
	return c.client.APIForVersion(version)
}

func (c *WebClient) APIForSlot(index iotago.SlotIndex) iotago.API {
	return c.client.APIForSlot(index)
}

func (c *WebClient) APIForEpoch(index iotago.EpochIndex) iotago.API {
	return c.client.APIForEpoch(index)
}

func (c *WebClient) CurrentAPI() iotago.API {
	return c.client.CurrentAPI()
}

func (c *WebClient) LatestAPI() iotago.API {
	return c.client.LatestAPI()
}

// URL returns a client API Url.
func (c *WebClient) URL() string {
	return c.url
}

// NewWebClient creates Connector from provided iota-core API urls.
func NewWebClient(url string, opts ...options.Option[WebClient]) *WebClient {
	return options.Apply(&WebClient{
		url: url,
	}, opts, func(w *WebClient) {
		w.client, _ = nodeclient.New(w.url)
	})
}

func (c *WebClient) PostBlock(block *iotago.ProtocolBlock) (blockID iotago.BlockID, err error) {
	id, err := c.client.SubmitBlock(context.Background(), block)
	if err != nil {
		return
	}

	return id, nil
}

// PostData sends the given data (payload) by creating a block in the backend.
func (c *WebClient) PostData(data []byte) (blkID string, err error) {
	blockBuilder := builder.NewBasicBlockBuilder(c.client.LatestAPI())
	blockBuilder.IssuingTime(time.Time{})

	blockBuilder.Payload(&iotago.TaggedData{
		Tag: data,
	})

	blk, err := blockBuilder.Build()
	if err != nil {
		return iotago.EmptyBlockID.ToHex(), err
	}

	id, err := c.client.SubmitBlock(context.Background(), blk)
	if err != nil {
		return
	}

	return id.ToHex(), nil
}

// GetOutputConfirmationState gets the first unspent outputs of a given address.
func (c *WebClient) GetOutputConfirmationState(outputID iotago.OutputID) string {
	txID := outputID.TransactionID()

	return c.GetTransactionConfirmationState(txID)
}

// GetOutput gets the output of a given outputID.
func (c *WebClient) GetOutput(outputID iotago.OutputID) iotago.Output {
	res, err := c.client.OutputByID(context.Background(), outputID)
	if err != nil {
		return nil
	}

	return res
}

// GetTransactionConfirmationState returns the AcceptanceState of a given transaction ID.
func (c *WebClient) GetTransactionConfirmationState(txID iotago.TransactionID) string {
	resp, err := c.client.TransactionIncludedBlockMetadata(context.Background(), txID)
	if err != nil {
		return ""
	}

	return resp.TxState
}

// GetTransaction gets the transaction.
func (c *WebClient) GetTransaction(txID iotago.TransactionID) (tx *iotago.SignedTransaction, err error) {
	resp, err := c.client.TransactionIncludedBlock(context.Background(), txID)
	if err != nil {
		return
	}

	modelBlk, err := model.BlockFromBlock(resp)
	if err != nil {
		return
	}

	tx, _ = modelBlk.SignedTransaction()

	return tx, nil
}

func (c *WebClient) GetBlockIssuance() (resp *apimodels.IssuanceBlockHeaderResponse, err error) {
	resp, err = c.client.BlockIssuance(context.Background())
	if err != nil {
		return
	}

	return
}

func (c *WebClient) GetCongestion(accountID iotago.AccountID) (resp *apimodels.CongestionResponse, err error) {
	resp, err = c.client.Congestion(context.Background(), accountID)
	if err != nil {
		return
	}

	return
}
