import * as React from 'react';
import {hot} from 'react-hot-loader/root';
import {Root} from './components/Root';

// render react DOM
export const App = hot(({history}) => <Root history={history}/>);
