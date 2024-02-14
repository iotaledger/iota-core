package debugapi

import (
	"bytes"
	"fmt"

	"github.com/goccy/go-graphviz"
	"github.com/goccy/go-graphviz/cgraph"

	"github.com/iotaledger/hive.go/ds/walker"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/iota-core/pkg/protocol"
)

func chainManagerAllChainsDot() (string, error) {
	rootCommitment := deps.Protocol.Chains.Main.Get().ForkingPoint.Get()
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
	rootCommitment := deps.Protocol.Chains.Main.Get().ForkingPoint.Get()
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

func prepareCommitmentGraph(g *graphviz.Graphviz, rootCommitment *protocol.Commitment) (*cgraph.Graph, error) {
	graph, err := g.Graph()
	if err != nil {
		return nil, err
	}

	root, rootErr := createNode(graph, rootCommitment)
	if rootErr != nil {
		return nil, rootErr
	}
	root.SetColor("green")

	for commitmentWalker := walker.New[*protocol.Commitment](false).Push(rootCommitment); commitmentWalker.HasNext(); {
		parentCommitment := commitmentWalker.Next()
		parent, parentErr := createNode(graph, parentCommitment)
		if parentErr != nil {
			return nil, parentErr
		}

		// TODO: this should be removed once eviction of commitments is properly implemented
		if parentCommitment.Children.IsEmpty() {
			if childCommitment, exists := parentCommitment.Chain.Get().Commitment(parentCommitment.Slot() + 1); exists {
				if err = renderChild(childCommitment, graph, parentCommitment, parent); err != nil {
					return nil, err
				}

				commitmentWalker.Push(childCommitment)

				continue
			}
		}

		if err = parentCommitment.Children.ForEach(func(childCommitment *protocol.Commitment) error {
			if err = renderChild(childCommitment, graph, parentCommitment, parent); err != nil {
				return err
			}

			commitmentWalker.Push(childCommitment)

			return nil
		}); err != nil {
			return nil, err
		}
	}

	return graph, nil
}

func renderChild(childCommitment *protocol.Commitment, graph *cgraph.Graph, parentCommitment *protocol.Commitment, parent *cgraph.Node) error {
	child, err := createNode(graph, childCommitment)
	if err != nil {
		return err
	}

	if childCommitment.Chain.Get() == deps.Protocol.Chains.Main.Get() {
		child.SetColor("green")
	}

	if _, edgeErr := graph.CreateEdge(fmt.Sprintf("%s -> %s", parentCommitment.ID().String()[:8], childCommitment.ID().String()[:8]), parent, child); edgeErr != nil {
		return ierrors.Wrapf(edgeErr, "could not create edge %s -> %s", parentCommitment.ID().String()[:8], childCommitment.ID().String()[:8])
	}

	return nil
}

func createNode(graph *cgraph.Graph, commitment *protocol.Commitment) (*cgraph.Node, error) {
	node, err := graph.CreateNode(fmt.Sprintf("%d-%s", commitment.ID().Slot(), commitment.ID().Identifier().String()[:8]))
	if err != nil {
		return nil, ierrors.Wrapf(err, "could not retrieve node %s", commitment.ID().Identifier().String()[:8])
	}
	if node != nil {
		return node, nil
	}

	node, err = graph.CreateNode(fmt.Sprintf("%d-%s", commitment.ID().Slot(), commitment.ID().Identifier().String()[:8]))
	if err != nil {
		return nil, ierrors.Wrapf(err, "could not create node %s", commitment.ID().Identifier().String()[:8])
	}

	return node, nil
}
