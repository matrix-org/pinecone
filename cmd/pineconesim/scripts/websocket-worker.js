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
        this.state.socket = new WebSocket(this.state.url);
        this.state.socket.onopen = this.socketOpenListener;
        this.state.socket.onmessage = this.socketMessageListener;
        this.state.socket.onclose = this.socketCloseListener;
        this.state.socket.onerror = this.socketErrorListener;
    }
}

onmessage = e => {
    if (e.data.hasOwnProperty('url')){
        websocketWorker = new WebsocketWorker(e.data.url);
    }
};
