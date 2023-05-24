import * as React from 'react';
import Container from "react-bootstrap/Container";
import Row from "react-bootstrap/Row";
import Col from "react-bootstrap/Col";
import NodeStore from "../stores/NodeStore";
import {inject, observer} from "mobx-react";
import {ExplorerSearchbar} from "./ExplorerSearchbar";
import {ExplorerLiveFeed} from "./ExplorerLiveFeed";
import {ExplorerTransactionSearchbar} from "./ExplorerTransactionSearchbar";
import {ExplorerOutputSearchbar} from "./ExplorerOutputSearchbar";
import {ExplorerConflictSearchbar} from "./ExplorerConflictSearchbar";

interface Props {
    nodeStore?: NodeStore;
}

@inject("nodeStore")
@observer
export class Explorer extends React.Component<Props, any> {
    render() {
        return (
            <Container>
                <h3>Tangle Explorer</h3>
                <Row className={"mb-3"}>
                    <Col>
                        <p>
                            Search for addresses, blocks, transactions, outputs and conflicts.
                        </p>
                    </Col>
                </Row>
                <Row>
                    <Col>
                        <ExplorerSearchbar/>
                    </Col>
                    <Col>
                        <ExplorerTransactionSearchbar/>
                    </Col>
                </Row>
                <Row>
                    <Col>
                        <ExplorerOutputSearchbar/>
                    </Col>
                    <Col>
                        <ExplorerConflictSearchbar/>
                    </Col>
                </Row>
                <ExplorerLiveFeed/>
                <small>
                </small>
            </Container>
        );
    }
}
