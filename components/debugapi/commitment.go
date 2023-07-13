package debugapi

import (
	"bytes"
	"fmt"

	"github.com/goccy/go-graphviz"
	"github.com/goccy/go-graphviz/cgraph"

	"github.com/iotaledger/hive.go/ds/walker"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/protocol/chainmanager"
)

func chainManagerAllChainsDot() (string, error) {
	rootCommitment := deps.Protocol.ChainManager.RootCommitment()
	g := graphviz.New()
	defer g.Close()

	graph, err := prepareCommitmentGraph(g, rootCommitment)
	if err != nil {
		return "", err
	}
	defer graph.Close()

	var buf bytes.Buffer
	if err := g.Render(graph, "dot", &buf); err != nil {
		return "", err
	}

	return buf.String(), nil
}

func chainManagerAllChainsRendered() ([]byte, error) {
	rootCommitment := deps.Protocol.ChainManager.RootCommitment()
	g := graphviz.New()
	defer g.Close()

	graph, err := prepareCommitmentGraph(g, rootCommitment)
	if err != nil {
		return nil, err
	}
	defer graph.Close()

	var buf bytes.Buffer
	if err := g.Render(graph, graphviz.PNG, &buf); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func prepareCommitmentGraph(g *graphviz.Graphviz, rootCommitment *chainmanager.ChainCommitment) (*cgraph.Graph, error) {
	graph := lo.PanicOnErr(g.Graph())

	_, rootErr := graph.CreateNode(fmt.Sprintf("%d: %s", rootCommitment.ID().Index(), rootCommitment.ID().String()[:8]))
	if rootErr != nil {
		return nil, ierrors.Wrapf(rootErr, "could not create root node %s", rootCommitment.ID().String()[:8])
	}

	for commitmentWalker := walker.New[*chainmanager.ChainCommitment](false).Push(rootCommitment); commitmentWalker.HasNext(); {
		parentCommitment := commitmentWalker.Next()
		parent, parentErr := graph.Node(fmt.Sprintf("%d: %s", parentCommitment.ID().Index(), parentCommitment.ID().String()[:8]))
		if parentErr != nil {
			return nil, ierrors.Wrapf(parentErr, "could not create parent node %s", parentCommitment.ID().String()[:8])
		}

		for _, childCommitment := range parentCommitment.Children() {
			child, childErr := graph.CreateNode(fmt.Sprintf("%d: %s", childCommitment.ID().Index(), childCommitment.ID().String()[:8]))
			if childCommitment.Chain().ForkingPoint.ID() == deps.Protocol.MainEngineInstance().ChainID() {
				child.SetColor("green")
			}

			if childErr != nil {
				return nil, ierrors.Wrapf(childErr, "could not create child node %s", childCommitment.ID().String()[:8])
			}

			if _, edgeErr := graph.CreateEdge(fmt.Sprintf("%s -> %s", parentCommitment.ID().String()[:8], childCommitment.ID().String()[:8]), parent, child); edgeErr != nil {
				return nil, ierrors.Wrapf(edgeErr, "could not create edge %s -> %s", parentCommitment.ID().String()[:8], childCommitment.ID().String()[:8])
			}

			commitmentWalker.Push(childCommitment)

		}
	}

	return graph, nil
}
