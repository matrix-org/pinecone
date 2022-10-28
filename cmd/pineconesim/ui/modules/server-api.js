export const APIEventMessageID = {
    Unknown: 0,
    InitialState: 1,
    StateUpdate: 2,
};

export const APICommandMessageID = {
    Unknown: 0,
    PlaySequence: 1,
};

export const APIUpdateID = {
    Unknown: 0,
    NodeAdded: 1,
    NodeRemoved: 2,
    PeerAdded: 3,
    PeerRemoved: 4,
    TreeParentUpdated: 5,
    SnakeAscUpdated: 6,
    SnakeDescUpdated: 7,
    TreeRootAnnUpdated: 8,
    SnakeEntryAdded: 9,
    SnakeEntryRemoved: 10,
    PingStateUpdated: 11,
    NetworkStatsUpdated: 12,
    BroadcastReceived: 13,
    BandwidthReport: 14,
};

export const APICommandID = {
    Unknown: 0,
    Debug: 1,
    Play: 2,
    Pause: 3,
    Delay: 4,
    AddNode: 5,
    RemoveNode: 6,
    AddPeer: 7,
    RemovePeer: 8,
    ConfigureAdversaryDefaults: 9,
    ConfigureAdversaryPeer: 10,
    StartPings: 11,
    StopPings: 12,
};

export const APINodeType = {
    Unknown: 0,
    Default: 1,
    GeneralAdversary: 2,
};

var serverWorker;

export function ConnectToServer(url, handler) {
    if (!serverWorker) {
        console.log("Connecting to server at: " + url.url);
        serverWorker = new Worker("ui/websocket-worker.js");
        serverWorker.onmessage = handler;
        serverWorker.postMessage(url);
    }
}

export function SendToServer(msg) {
    if (serverWorker) {
        serverWorker.postMessage(msg);
    }
}

export function ConvertNodeTypeToString(nodeType) {
    let val = "Unknown";
    switch(nodeType) {
    case APINodeType.Default:
        val = "Default";
        break;
    case APINodeType.GeneralAdversary:
        val = "General Adversary";
        break;
    }

    return val;
}
