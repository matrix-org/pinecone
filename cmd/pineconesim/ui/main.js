import { graph } from "./modules/graph.js";
import "./modules/ui.js";

const worker = new Worker("ui/websocket-worker.js");

const APIEventMessageID = {
    Unknown: 0,
    InitialState: 1,
    StateUpdate: 2,
};

const APICommandMessageID = {
    Unknown: 0,
    PlaySequence: 1,
};

const APIUpdateID = {
    Unknown: 0,
    NodeAdded: 1,
    NodeRemoved: 2,
    PeerAdded: 3,
    PeerRemoved: 4,
    TreeParentUpdated: 5,
    SnakeAscUpdated: 6,
    SnakeDescUpdated: 7,
    TreeRootAnnUpdated: 8,
};

const APICommandID = {
    Unknown: 0,
    Debug: 1,
    Delay: 2,
    AddNode: 3,
    RemoveNode: 4,
    AddPeer: 5,
    RemovePeer: 6,
};

function handleSimMessage(msg) {
    // console.log(msg.data);
    switch(msg.data.MsgID) {
    case APIEventMessageID.InitialState:
        for (let [key, value] of Object.entries(msg.data.Nodes)) {
            graph.addNode(key, value.PublicKey);
            graph.updateRootAnnouncement(key, value.RootState.Root, value.RootState.AnnSequence, value.RootState.AnnTime, value.RootState.Coords);

            if (value.Peers) {
                for (let i = 0; i < value.Peers.length; i++) {
                    graph.addPeer(key, value.Peers[i].ID, value.Peers[i].Port);
                }
            }

            if (value.SnakeAsc && value.SnakeAscPath) {
                graph.setSnekAsc(key, value.SnakeAsc, "", value.SnakeAscPath);
            }
            if (value.SnakeDesc && value.SnakeDescPath) {
                graph.setSnekDesc(key, value.SnakeDesc, "", value.SnakeDescPath);
            }

            if (value.TreeParent) {
                graph.setTreeParent(key, value.TreeParent, "");
            }
        }

        if (msg.data.End === true) {
            graph.startGraph();
        }
        break;
    case APIEventMessageID.StateUpdate:
        let event = msg.data.Event.Event;
        switch(msg.data.Event.UpdateID) {
        case APIUpdateID.NodeAdded:
            graph.addNode(event.Node, event.PublicKey);
            break;
        case APIUpdateID.NodeRemoved:
            graph.removeNode(event.Node);
            break;
        case APIUpdateID.PeerAdded:
            graph.addPeer(event.Node, event.Peer, event.Port);
            break;
        case APIUpdateID.PeerRemoved:
            graph.removePeer(event.Node, event.Peer);
            break;
        case APIUpdateID.TreeParentUpdated:
            graph.setTreeParent(event.Node, event.Peer, event.Prev);
            break;
        case APIUpdateID.SnakeAscUpdated:
            graph.setSnekAsc(event.Node, event.Peer, event.Prev, event.PathID);
            break;
        case APIUpdateID.SnakeDescUpdated:
            graph.setSnekDesc(event.Node, event.Peer, event.Prev, event.PathID);
            break;
        case APIUpdateID.TreeRootAnnUpdated:
            graph.updateRootAnnouncement(event.Node, event.Root, event.Sequence, event.Time, event.Coords);
        }
        break;
    default:
        console.log("Unhandled message ID");
        break;
    }
};

worker.onmessage = handleSimMessage;

// Start the websocket worker with the current url
worker.postMessage({url: window.origin.replace("http", "ws") + '/ws'});
