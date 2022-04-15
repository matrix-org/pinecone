import { graph } from "./modules/graph.js";
import { APIEventMessageID, APICommandMessageID, APIUpdateID, APICommandID,
         ConnectToServer, SendToServer } from "./modules/server-api.js";
import { SetPingToolState } from "./modules/ui.js";

function handleSimMessage(msg) {
    // console.log(msg.data);
    switch(msg.data.MsgID) {
    case APIEventMessageID.InitialState:
        for (let [key, value] of Object.entries(msg.data.Nodes)) {
            graph.addNode(key, value.PublicKey, value.NodeType);
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
            console.log("Finished receiving initial state from server. Listening for further updates...");
            graph.startGraph();
        }
        break;
    case APIEventMessageID.StateUpdate:
        let event = msg.data.Event.Event;
        switch(msg.data.Event.UpdateID) {
        case APIUpdateID.NodeAdded:
            graph.addNode(event.Node, event.PublicKey, event.NodeType);
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
        case APIUpdateID.PingStateUpdated:
            SetPingToolState(event.Active);
        }
        break;
    default:
        console.log("Unhandled message ID: " + msg.data.MsgID);
        break;
    }
};

ConnectToServer({url: window.origin.replace("http", "ws") + '/ws'}, handleSimMessage);

function selectStartingNetwork() {
    let selectionTabs = document.getElementsByClassName("netselect");
    var hash = window.location.hash.substr(1);
    if (hash != "") {
        for (let i = 0; i < selectionTabs.length; i++) {
            if (selectionTabs[i].id.includes(hash)) {
                selectionTabs[i].click();
                break;
            }
        }
    }
}
selectStartingNetwork();
