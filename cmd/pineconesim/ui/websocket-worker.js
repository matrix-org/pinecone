var websocketWorker;

class WebsocketWorker {
    state = {
        url: "",
        socket: null,
    };

    constructor(url) {
        this.state = {
            url: url,
        };

        this.socketCloseListener();
    }

    socketMessageListener(e) {
        let msg = JSON.parse(e.data);
        postMessage(msg);
    }

    socketOpenListener(e) {
        console.log('Connected');
    }

    socketErrorListener(e) {
        console.error(e);
    }

    socketCloseListener(e) {
        if (this.state && this.state.socket) {
            console.log('Disconnected.');
        }

        if (this.state) {
            this.state.socket = new WebSocket(this.state.url);
            this.state.socket.onopen = this.socketOpenListener;
            this.state.socket.onmessage = this.socketMessageListener;
            this.state.socket.onclose = this.socketCloseListener;
            this.state.socket.onerror = this.socketErrorListener;
        }
    }

    SendCommandSequence(data) {
        if (this.state.socket) {
            console.log("Sending commands to sim: " + JSON.stringify(data));
            this.state.socket.send(JSON.stringify(data));
        }
    }
}

onmessage = e => {
    if (e.data.hasOwnProperty('url')){
        websocketWorker = new WebsocketWorker(e.data.url);
    } else if (e.data.hasOwnProperty('Events')){
        websocketWorker.SendCommandSequence(e.data);
    }
};
