let websocketWorker;
let serverUrl;
let connectedToServer = false;

class WebsocketWorker {
    state = {
        socket: null,
    };

    constructor(url) {
        this.state = {
            socket: null,
        };

        serverUrl = url;
        connectedToServer = false;
    }

    Connect() {
        this.socketConnect();
    }

    socketConnect() {
        if (this.state) {
            this.state.socket = new WebSocket(serverUrl);
            this.state.socket.onopen = this.socketOpenListener;
            this.state.socket.onmessage = this.socketMessageListener;
            this.state.socket.onclose = this.socketCloseListener;
            this.state.socket.onerror = this.socketErrorListener;
        }
    }

    socketMessageListener(e) {
        let msg = JSON.parse(e.data);
        postMessage(msg);
    }

    socketOpenListener(e) {
        console.log('Connected to server');
        connectedToServer = true;
    }

    socketErrorListener(e) {
        console.error(e);
        connectedToServer = false;
    }

    socketCloseListener(e) {
        if (connectedToServer) {
            console.log('Disconnected from server');
            connectedToServer = false;
        }
    }

    SendCommandSequence(data) {
        if (this.state.socket && connectedToServer) {
            console.log("Sending commands to server: " + JSON.stringify(data));
            this.state.socket.send(JSON.stringify(data));
        } else {
            console.error("Failed sending command to server: Not connected");
        }
    }
}

onmessage = e => {
    if (e.data.hasOwnProperty('url')){
        websocketWorker = new WebsocketWorker(e.data.url);
        websocketWorker.Connect();
    } else if (e.data.hasOwnProperty('Events')){
        websocketWorker.SendCommandSequence(e.data);
    } else {
        console.error('Received unhandled message: ' + e);
    }
};
