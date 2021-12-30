import { graph } from "./graph.js";
import { APICommandMessageID, APICommandID, SendToServer } from "./server-api.js";

let leftShown = true;
let rightShown = false;

export function openRightPanel() {
    if (!rightShown) {
        toggleRightPanel();
    }
}

export function closeRightPanel() {
    if (rightShown) {
        toggleRightPanel();
    }
}

function toggleLeftPanel() {
    let panel = document.getElementById("left");

    if (panel) {
        if (!leftShown) {
            panel.style.width = "max-content";
            leftShown= true;
        } else {
            panel.style.width = "0";
            leftShown= false;
        }
    }
}
document.getElementById("leftToggle").onclick = toggleLeftPanel;

function toggleRightPanel() {
    let panel = document.getElementById("right");

    if (panel) {
        if (!rightShown) {
            panel.style.width = "max-content";
            let toggleSize = parseInt(getComputedStyle(document.documentElement)
                                      .getPropertyValue('--toggle-box-size'), 10);
            let toggleSizePadding = parseInt(getComputedStyle(document.documentElement)
                                             .getPropertyValue('--toggle-box-margin'), 10);

            panel.style.minWidth = (toggleSize + toggleSizePadding * 2).toString() + "px";
            rightShown= true;
        } else {
            panel.style.width = "0";
            panel.style.minWidth = "0vw";
            rightShown= false;
        }
    }
}
document.getElementById("rightToggle").onclick = toggleRightPanel;

function focusSelectedNode() {
    let button = document.getElementById("focusNode");
    if (button) {
        graph.focusSelectedNode();
    }
}
document.getElementById("focusNode").onclick = focusSelectedNode;

function selectNetworkType(networkType) {
    let selectionTabs = document.getElementsByClassName("netselect");
    for (let i = 0; i < selectionTabs.length; i++) {
        selectionTabs[i].className = selectionTabs[i].className.replace(" active", "");
    }

    this.className += " active";

    switch(this.id) {
    case "peerTopo":
        graph.changeDataSet("peer");
        break;
    case "snakeTopo":
        graph.changeDataSet("snake");
        break;
    case "treeTopo":
        graph.changeDataSet("tree");
        break;
    case "geographicTopo":
        graph.changeDataSet("geographic");
        break;
    }
}

function setupNetworkSelection() {
    let selectionTabs = document.getElementsByClassName("netselect");
    for (let i = 0; i < selectionTabs.length; i++) {
        selectionTabs[i].onclick = selectNetworkType;
    }
}
setupNetworkSelection();

function selectTool(toolType) {
    let selectionTabs = document.getElementsByClassName("toolselect");
    for (let i = 0; i < selectionTabs.length; i++) {
        selectionTabs[i].className = selectionTabs[i].className.replace(" active", "");
    }

    this.className += " active";

    switch(this.id) {
    case "scenarioSelect":
        console.log(this.id);
        // TODO
        break;
    case "create":
        console.log(this.id);
        // TODO
        break;
    case "remove":
        console.log(this.id);
        // TODO : do peer connections as well (but not tree or snake)
        let commands = [];
        let nodes = graph.GetSelectedNodes();
        if (nodes) {
            for (let i = 0; i < nodes.length; i++) {
                commands.push({"MsgID": APICommandID.RemoveNode, "Event": {"Name": nodes[i]}});
            }
        }

        if (commands.length > 0) {
            SendToServer({"MsgID": APICommandMessageID.PlaySequence, "Events": commands});
        }
        // TODO : the remove button should probably flash temporarily to active
        this.className = this.className.replace(" active", "");
        break;
    case "capture":
        console.log(this.id);
        // TODO
        break;
    case "replay":
        console.log(this.id);
        // TODO
        break;
    }
}

function setupToolSelection() {
    let selectionTabs = document.getElementsByClassName("toolselect");
    for (let i = 0; i < selectionTabs.length; i++) {
        selectionTabs[i].onclick = selectTool;
    }
}
setupToolSelection();
