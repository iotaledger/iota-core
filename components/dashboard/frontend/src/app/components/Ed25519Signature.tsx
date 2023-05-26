import * as React from 'react';
import Row from "react-bootstrap/Row";
import { Col, ListGroup } from 'react-bootstrap';
import { SignatureEd25519 } from 'app/misc/Payload';

interface Props {
    signature?: SignatureEd25519;
}

export class Ed25519Signature extends React.Component<Props, any> {

    render() {
        let signature = this.props.signature;
        return (
            <React.Fragment>
                <Row className={"mb-3"}>
                    <Col>
                        <ListGroup>
                            <ListGroup.Item>Public Key: {signature.publicKey}</ListGroup.Item>
                            <ListGroup.Item>Signature: {signature.signature}</ListGroup.Item>
                        </ListGroup>
                    </Col>
                </Row>
            </React.Fragment>
        );
    }
}
