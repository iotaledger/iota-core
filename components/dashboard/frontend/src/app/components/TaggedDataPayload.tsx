import * as React from 'react';
import Row from "react-bootstrap/Row";
import {inject, observer} from "mobx-react";
import ExplorerStore from "../stores/ExplorerStore";
import { Col, ListGroup } from 'react-bootstrap';

interface Props {
    explorerStore?: ExplorerStore;
}

@inject("explorerStore")
@observer
export class TaggedDataPayload extends React.Component<Props, any> {

    render() {
        let {payload} = this.props.explorerStore;
        
        return (
            payload &&
            <React.Fragment>
                <Row className={"mb-3"}>
                    <Col>
                        <ListGroup>
                            <ListGroup.Item>Tag: {payload.tag}</ListGroup.Item>
                            <ListGroup.Item>Data: {payload.data}</ListGroup.Item>
                        </ListGroup>
                    </Col>
                </Row>
            </React.Fragment>
        );
    }
}
