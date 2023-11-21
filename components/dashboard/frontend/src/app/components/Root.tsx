import * as React from 'react';
import {inject, observer} from "mobx-react";
import NodeStore from "../stores/NodeStore";
import Navbar from "react-bootstrap/Navbar";
import Nav from "react-bootstrap/Nav";
import {Dashboard} from "./Dashboard";
import Badge from "react-bootstrap/Badge";
import {RouterStore} from 'mobx-react-router';
import {Explorer} from "./Explorer";
import {NavExplorerSearchbar} from "./NavExplorerSearchbar";
import {Redirect, Route, Switch} from 'react-router-dom';
import {LinkContainer} from 'react-router-bootstrap';
import {ExplorerBlockQueryResult} from "./ExplorerBlockQueryResult";
import {ExplorerAddressQueryResult} from "./ExplorerAddressResult";
import {Explorer404} from "./Explorer404";
import {Neighbors} from "./Neighbors";
import {Visualizer} from "./Visualizer";
import {Tips} from "./Tips";
import {ExplorerTransactionQueryResult} from "./ExplorerTransactionQueryResult";
import {ExplorerOutputQueryResult} from "./ExplorerOutputQueryResult";
import {ExplorerSpendQueryResult} from "./ExplorerSpendQueryResult";
import { SlotLiveFeed } from './SlotLiveFeed';
import { ExplorerSlotQueryResult } from './ExplorerSlotQueryResult';

interface Props {
    history: any;
    routerStore?: RouterStore;
    nodeStore?: NodeStore;
}

@inject("nodeStore")
@inject("routerStore")
@observer
export class Root extends React.Component<Props, any> {
    renderDevTool() {
        if (process.env.NODE_ENV !== 'production') {
            const DevTools = require('mobx-react-devtools').default;
            return <DevTools/>;
        }
    }

    componentDidMount(): void {
        this.props.nodeStore.connect();
    }

    render() {
        return (
            <div className="container">
                <Navbar expand="lg" bg="light" variant="light" className={"mb-4"}>
                    <Navbar.Brand>iota-core</Navbar.Brand>
                    <Nav className="mr-auto">
                        <LinkContainer to="/dashboard">
                            <Nav.Link>Dashboard</Nav.Link>
                        </LinkContainer>
                        <LinkContainer to="/neighbors">
                            <Nav.Link>Neighbors</Nav.Link>
                        </LinkContainer>
                        <LinkContainer to="/explorer">
                            <Nav.Link>
                                Explorer
                            </Nav.Link>
                        </LinkContainer>
                        <LinkContainer to="/visualizer">
                            <Nav.Link>
                                Visualizer
                            </Nav.Link>
                        </LinkContainer>
                        <LinkContainer to="/slots">
                            <Nav.Link>
                                Slot
                            </Nav.Link>
                        </LinkContainer>
                        <LinkContainer to="/tips">
                            <Nav.Link>
                                Tips
                            </Nav.Link>
                        </LinkContainer>
                    </Nav>
                    <Navbar.Collapse className="justify-content-end">
                        <NavExplorerSearchbar/>
                        <Navbar.Text>
                            {!this.props.nodeStore.websocketConnected &&
                            <Badge variant="danger">WS not connected!</Badge>
                            }
                        </Navbar.Text>
                    </Navbar.Collapse>
                </Navbar>
                <Switch>
                    <Route exact path="/dashboard" component={Dashboard}/>
                    <Route exact path="/neighbors" component={Neighbors}/>
                    <Route exact path="/explorer/block/:id" component={ExplorerBlockQueryResult}/>
                    <Route exact path="/explorer/address/:id" component={ExplorerAddressQueryResult}/>
                    <Route exact path="/explorer/transaction/:id" component={ExplorerTransactionQueryResult}/>
                    <Route exact path="/explorer/output/:id" component={ExplorerOutputQueryResult}/>
                    <Route exact path="/explorer/spend/:id" component={ExplorerSpendQueryResult}/>
                    <Route exact path="/explorer/slot/commitment/:commitment" component={ExplorerSlotQueryResult}/>
                    <Route exact path="/explorer/404/:search" component={Explorer404}/>
                    <Route exact path="/slots" component={SlotLiveFeed}/>
                    <Route exact path="/tips" component={Tips}/>
                    <Route exact path="/explorer" component={Explorer}/>
                    <Route exact path="/visualizer" component={Visualizer}/>
                    <Route exact path="/visualizer/history" component={Visualizer}/>
                    <Redirect to="/dashboard"/>
                </Switch>
                {this.props.children}
                {this.renderDevTool()}
            </div>
        );
    }
}
